package client

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"time"

	"code.cloudfoundry.org/lager"
	"github.com/cloudfoundry/streaming-mysql-backup-client/clock"
	"github.com/cloudfoundry/streaming-mysql-backup-client/config"
	"github.com/cloudfoundry/streaming-mysql-backup-client/download"
	"github.com/cloudfoundry/streaming-mysql-backup-client/prepare"
	"github.com/cloudfoundry/streaming-mysql-backup-client/tarpit"

	"errors"
	"github.com/cloudfoundry/streaming-mysql-backup-client/cryptkeeper"
	"github.com/cloudfoundry/streaming-mysql-backup-client/fileutils"
	"github.com/cloudfoundry/streaming-mysql-backup-client/galera_agent_caller"
)

type MultiError []error

func (e MultiError) Error() string {
	var buf bytes.Buffer

	if len(e) > 1 {
		buf.WriteString("multiple errors:")
	}
	for _, err := range e {
		buf.WriteString("\n")
		buf.WriteString(err.Error())
	}

	return buf.String()
}

//go:generate counterfeiter . Downloader
type Downloader interface {
	DownloadBackup(url string, streamer download.StreamedWriter) error
}

//go:generate counterfeiter . BackupPreparer
type BackupPreparer interface {
	Command(string) *exec.Cmd
}

//go:generate counterfeiter . GaleraAgentCallerInterface
type GaleraAgentCallerInterface interface {
	WsrepLocalIndex(string) (int, error)
}

type Client struct {
	config            config.Config
	version           int64
	tarClient         *tarpit.TarClient
	backupPreparer    BackupPreparer
	downloader        Downloader
	galeraAgentCaller GaleraAgentCallerInterface
	logger            lager.Logger
	downloadDirectory string
	prepareDirectory  string
	encryptDirectory  string
	encryptor         *cryptkeeper.CryptKeeper
	metadataFields    map[string]string
}

func DefaultClient(config config.Config) *Client {
	return NewClient(
		config,
		tarpit.NewSystemTarClient(),
		prepare.DefaultBackupPreparer(),
		download.DefaultDownloadBackup(clock.DefaultClock(), config),
		galera_agent_caller.DefaultGaleraAgentCaller(config.GaleraAgentPort),
	)
}

func NewClient(config config.Config, tarClient *tarpit.TarClient, backupPreparer BackupPreparer, downloader Downloader, galeraAgentCaller GaleraAgentCallerInterface) *Client {
	client := &Client{
		config:            config,
		tarClient:         tarClient,
		backupPreparer:    backupPreparer,
		downloader:        downloader,
		galeraAgentCaller: galeraAgentCaller,
	}
	client.logger = config.Logger
	client.encryptor = cryptkeeper.NewCryptKeeper(config.SymmetricKey)
	client.metadataFields = config.MetadataFields
	return client
}

func (this Client) artifactName(index int) string {
	return fmt.Sprintf("mysql-backup-%d-%d", this.version, index)
}

func (this Client) downloadedBackupLocation() string {
	return path.Join(this.downloadDirectory, "unprepared-backup.tar")
}

func (this Client) preparedBackupLocation() string {
	return path.Join(this.encryptDirectory, "prepared-backup.tar")
}

func (this Client) encryptedBackupLocation(index int) string {
	return path.Join(this.config.OutputDir, fmt.Sprintf("%s.tar.gpg", this.artifactName(index)))
}

func (this Client) originalMetadataLocation() string {
	return path.Join(this.prepareDirectory, "xtrabackup_info")
}

func (this Client) finalMetadataLocation(index int) string {
	return path.Join(this.config.OutputDir, fmt.Sprintf("%s.txt", this.artifactName(index)))
}

func (this *Client) Execute() error {
	var allErrors MultiError
	var ips []string

	err := this.cleanTmpDirectories()
	if err != nil {
		return err
	}

	if this.config.BackupAllMasters {
		ips = this.config.Ips
	} else if this.config.BackupFromInactiveNode {
		var largestIndexHealthy int
		var largestIndexHealthyIp string

		for _, ip := range this.config.Ips {
			wsrepIndex, err := this.galeraAgentCaller.WsrepLocalIndex(ip)
			if err != nil {
				this.logError(ip, "Fetching node status from galera agent failed", err)
			}
			if wsrepIndex >= largestIndexHealthy {
				largestIndexHealthy = wsrepIndex
				largestIndexHealthyIp = ip
			}
		}

		if largestIndexHealthyIp == "" {
			return errors.New("No healthy nodes found")
		}

		ips = []string{largestIndexHealthyIp}
	} else {
		ips = []string{this.config.Ips[len(this.config.Ips)-1]}
	}
	for index, ip := range ips {
		this.version = time.Now().Unix()

		err := this.BackupNode(ip, index)
		if err != nil {
			allErrors = append(allErrors, err)
		}
		this.cleanDirectories(ip) //ensure directories are cleaned on error
	}

	if len(allErrors) == len(ips) {
		return allErrors
	}
	return nil
}

func (this *Client) BackupNode(ip string, index int) error {
	var err error
	err = this.createDirectories(ip)
	if err != nil {
		return err
	}
	err = this.downloadAndUntarBackup(ip)
	if err != nil {
		return err
	}
	err = this.prepareBackup(ip)
	if err != nil {
		return err
	}
	err = this.writeMetadataFile(ip, index)
	if err != nil {
		return err
	}
	err = this.tarAndEncryptBackup(ip, index)
	if err != nil {
		return err
	}
	return nil
}

func (this *Client) logError(ip string, action string, err error, data ...lager.Data) {
	extraData := lager.Data{
		"ip": ip,
	}
	data = append(data, extraData)
	this.logger.Error(action, err, data...)
}

func (this *Client) logInfo(ip string, action string, data ...lager.Data) {
	extraData := lager.Data{
		"ip": ip,
	}
	data = append(data, extraData)
	this.logger.Info(action, data...)
}

func (this *Client) logDebug(ip string, action string, data ...lager.Data) {
	extraData := lager.Data{
		"ip": ip,
	}
	data = append(data, extraData)
	this.logger.Debug(action, data...)
}

func (this *Client) createDirectories(ip string) error {
	this.logDebug(ip, "Creating directories")

	var err error
	this.downloadDirectory, err = ioutil.TempDir(this.config.TmpDir, "mysql-backup-downloads")
	if err != nil {
		this.logError(ip, "Error creating temporary directory 'mysql-backup-downloads'", err)
		return err
	}

	this.prepareDirectory, err = ioutil.TempDir(this.config.TmpDir, "mysql-backup-prepare")
	if err != nil {
		this.logError(ip, "Error creating temporary directory 'mysql-backup-prepare'", err)
		return err
	}

	this.encryptDirectory, err = ioutil.TempDir(this.config.TmpDir, "mysql-backup-encrypt")
	if err != nil {
		this.logError(ip, "Error creating temporary directory 'mysql-backup-encrypt'", err)
		return err
	}

	this.logDebug(ip, "Created directories", lager.Data{
		"downloadDirectory": this.downloadDirectory,
		"prepareDirectory":  this.prepareDirectory,
		"encryptDirectory":  this.encryptDirectory,
	})

	return nil
}

func (this *Client) downloadAndUntarBackup(ip string) error {
	this.logInfo(ip, "Starting download of backup", lager.Data{
		"backup-prepare-path": this.prepareDirectory,
	})

	url := fmt.Sprintf("https://%s:%d/backup", ip, this.config.BackupServerPort)
	err := this.downloader.DownloadBackup(url, tarpit.NewUntarStreamer(this.prepareDirectory))
	if err != nil {
		this.logError(ip, "DownloadBackup failed", err)
		return err
	}

	this.logInfo(ip, "Finished downloading backup", lager.Data{
		"backup-prepare-path": this.prepareDirectory,
	})

	return nil
}

func (this *Client) prepareBackup(ip string) error {
	backupPrepare := this.backupPreparer.Command(this.prepareDirectory)
	this.logDebug(ip, "Backup prepare command", lager.Data{
		"command": backupPrepare,
		"args":    backupPrepare.Args,
	})

	this.logInfo(ip, "Starting prepare of backup", lager.Data{
		"prepareDirectory": this.prepareDirectory,
	})
	output, err := backupPrepare.CombinedOutput()
	if err != nil {
		this.logError(ip, "Preparing the backup failed", err, lager.Data{
			"output": output,
		})
		return err
	}
	this.logInfo(ip, "Successfully prepared a backup")

	return nil
}

// The xtrabackup_info file inside of the backup artifact contains relevant
// metadata information useful to operators, e.g. the effective backup time = `start_time`
//
// Copy this outside of the resultant re-compressed artifact so operators
// can glean this useful information without first downloading the large backup
//
// We had to add a sample xtrabackup_info file to the test fixture because of
// this concrete file dependency
//
// See: https://www.pivotaltracker.com/story/show/98994636
func (this *Client) writeMetadataFile(ip string, index int) error {
	src := this.originalMetadataLocation()
	dst := this.finalMetadataLocation(index)

	this.logInfo(ip, "Copying metadata file", lager.Data{
		"from": src,
		"to":   dst,
	})

	_, err := os.Create(dst)
	if err != nil {
		return err
	}

	backupMetadataMap, err := fileutils.ExtractFileFields(src)
	if err != nil {
		this.logError(ip, "Opening xtrabackup-info file failed", err)
		return err
	}

	for key, value := range this.metadataFields {
		backupMetadataMap[key] = value
	}

	for key, value := range backupMetadataMap {
		keyValLine := fmt.Sprintf("%s = %s", key, value)
		err = fileutils.WriteLineToFile(dst, keyValLine)
		if err != nil {
			this.logError(ip, "Writing metadata file failed", err)
			return err
		}
	}

	this.logInfo(ip, "Finished writing metadata file")

	return nil
}

func (this *Client) tarAndEncryptBackup(ip string, index int) error {
	this.logInfo(ip, "Starting encrypting backup")

	tarCmd := this.tarClient.Tar(this.prepareDirectory)

	encryptedFileWriter, err := os.Create(this.encryptedBackupLocation(index))
	if err != nil {
		this.logError(ip, "Error creating encrypted backup file", err)
		return err
	}
	defer encryptedFileWriter.Close()

	stdoutPipe, err := tarCmd.StdoutPipe()
	if err != nil {
		this.logError(ip, "Error attaching stdout to encryption", err)
		return err
	}

	if err := tarCmd.Start(); err != nil {
		this.logError(ip, "Error starting tar command", err)
		return err
	}

	if err := this.encryptor.Encrypt(stdoutPipe, encryptedFileWriter); err != nil {
		this.logError(ip, "Error while encrypting backup file", err)
		return err
	}

	if err := tarCmd.Wait(); err != nil {
		this.logError(ip, "Error while executing tar command", err)
		return err
	}

	this.logInfo(ip, "Successfully encrypted backup")
	return nil
}

func (this *Client) cleanDownloadDirectory(ip string) error {
	err := os.RemoveAll(this.downloadDirectory)
	if err != nil {
		this.logError(ip, fmt.Sprintf("Failed to remove %s", this.downloadDirectory), err)
		return err
	}

	this.logDebug(ip, "Cleaned download directory")
	return nil
}

func (this *Client) cleanPrepareDirectory(ip string) error {
	err := os.RemoveAll(this.prepareDirectory)
	if err != nil {
		this.logError(ip, fmt.Sprintf("Failed to remove %s", this.prepareDirectory), err)
		return err
	}

	this.logDebug(ip, "Cleaned prepare directory")
	return nil
}

func (this *Client) cleanEncryptDirectory(ip string) error {
	err := os.RemoveAll(this.encryptDirectory)
	if err != nil {
		this.logError(ip, fmt.Sprintf("Failed to remove %s", this.encryptDirectory), err)
		return err
	}

	this.logDebug(ip, "Cleaned encrypt directory")
	return nil
}

func (this *Client) cleanTmpDirectories() error {
	this.logger.Debug( "Cleaning tmp directory", lager.Data{
	  "tmpDirectory": this.config.TmpDir,
	})

	tmpDirs, err := filepath.Glob(filepath.Join(this.config.TmpDir, "mysql-backup*"))

	if err != nil {
		return err
	}

	for _, dir:= range(tmpDirs) {
		err = os.RemoveAll(dir)
		if err != nil {
			this.logger.Error(fmt.Sprintf("Failed to remove tmp directory %s", dir), err)
			return err
		}
	}

	this.logger.Debug("Cleaned tmp directory")

	return nil
}

func (this *Client) cleanDirectories(ip string) error {
	this.logDebug(ip, "Cleaning directories", lager.Data{
		"downloadDirectory": this.downloadDirectory,
		"prepareDirectory":  this.prepareDirectory,
		"encryptDirectory":  this.encryptDirectory,
	})

	//continue execution even if cleanup fails
	_ = this.cleanDownloadDirectory(ip)
	_ = this.cleanPrepareDirectory(ip)
	_ = this.cleanEncryptDirectory(ip)
	return nil
}

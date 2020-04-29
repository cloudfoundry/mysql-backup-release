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

func (c Client) artifactName(index int) string {
	return fmt.Sprintf("mysql-backup-%d-%d", c.version, index)
}

func (c Client) downloadedBackupLocation() string {
	return path.Join(c.downloadDirectory, "unprepared-backup.tar")
}

func (c Client) preparedBackupLocation() string {
	return path.Join(c.encryptDirectory, "prepared-backup.tar")
}

func (c Client) encryptedBackupLocation(index int) string {
	return path.Join(c.config.OutputDir, fmt.Sprintf("%s.tar.gpg", c.artifactName(index)))
}

func (c Client) originalMetadataLocation() string {
	return path.Join(c.prepareDirectory, "xtrabackup_info")
}

func (c Client) finalMetadataLocation(index int) string {
	return path.Join(c.config.OutputDir, fmt.Sprintf("%s.txt", c.artifactName(index)))
}

func (c *Client) Execute() error {
	var allErrors MultiError
	var ips []string

	err := c.cleanTmpDirectories()
	if err != nil {
		return err
	}

	if c.config.BackupAllMasters {
		ips = c.config.Ips
	} else if c.config.BackupFromInactiveNode {
		var largestIndexHealthy int
		var largestIndexHealthyIp string

		for _, ip := range c.config.Ips {
			wsrepIndex, err := c.galeraAgentCaller.WsrepLocalIndex(ip)
			if err != nil {
				c.logError(ip, "Fetching node status from galera agent failed", err)
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
		ips = []string{c.config.Ips[len(c.config.Ips)-1]}
	}
	for index, ip := range ips {
		c.version = time.Now().Unix()

		err := c.BackupNode(ip, index)
		if err != nil {
			allErrors = append(allErrors, err)
		}
		c.cleanDirectories(ip) //ensure directories are cleaned on error
	}

	if len(allErrors) == len(ips) {
		return allErrors
	}
	return nil
}

func (c *Client) BackupNode(ip string, index int) error {
	var err error
	err = c.createDirectories(ip)
	if err != nil {
		return err
	}
	err = c.downloadAndUntarBackup(ip)
	if err != nil {
		return err
	}
	err = c.prepareBackup(ip)
	if err != nil {
		return err
	}
	err = c.writeMetadataFile(ip, index)
	if err != nil {
		return err
	}
	err = c.tarAndEncryptBackup(ip, index)
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) logError(ip string, action string, err error, data ...lager.Data) {
	extraData := lager.Data{
		"ip": ip,
	}
	data = append(data, extraData)
	c.logger.Error(action, err, data...)
}

func (c *Client) logInfo(ip string, action string, data ...lager.Data) {
	extraData := lager.Data{
		"ip": ip,
	}
	data = append(data, extraData)
	c.logger.Info(action, data...)
}

func (c *Client) logDebug(ip string, action string, data ...lager.Data) {
	extraData := lager.Data{
		"ip": ip,
	}
	data = append(data, extraData)
	c.logger.Debug(action, data...)
}

func (c *Client) createDirectories(ip string) error {
	c.logDebug(ip, "Creating directories")

	var err error
	c.downloadDirectory, err = ioutil.TempDir(c.config.TmpDir, "mysql-backup-downloads")
	if err != nil {
		c.logError(ip, "Error creating temporary directory 'mysql-backup-downloads'", err)
		return err
	}

	c.prepareDirectory, err = ioutil.TempDir(c.config.TmpDir, "mysql-backup-prepare")
	if err != nil {
		c.logError(ip, "Error creating temporary directory 'mysql-backup-prepare'", err)
		return err
	}

	c.encryptDirectory, err = ioutil.TempDir(c.config.TmpDir, "mysql-backup-encrypt")
	if err != nil {
		c.logError(ip, "Error creating temporary directory 'mysql-backup-encrypt'", err)
		return err
	}

	c.logDebug(ip, "Created directories", lager.Data{
		"downloadDirectory": c.downloadDirectory,
		"prepareDirectory":  c.prepareDirectory,
		"encryptDirectory":  c.encryptDirectory,
	})

	return nil
}

func (c *Client) downloadAndUntarBackup(ip string) error {
	c.logInfo(ip, "Starting download of backup", lager.Data{
		"backup-prepare-path": c.prepareDirectory,
	})

	url := fmt.Sprintf("https://%s:%d/backup", ip, c.config.BackupServerPort)
	err := c.downloader.DownloadBackup(url, tarpit.NewUntarStreamer(c.prepareDirectory))
	if err != nil {
		c.logError(ip, "DownloadBackup failed", err)
		return err
	}

	c.logInfo(ip, "Finished downloading backup", lager.Data{
		"backup-prepare-path": c.prepareDirectory,
	})

	return nil
}

func (c *Client) prepareBackup(ip string) error {
	backupPrepare := c.backupPreparer.Command(c.prepareDirectory)
	c.logDebug(ip, "Backup prepare command", lager.Data{
		"command": backupPrepare,
		"args":    backupPrepare.Args,
	})

	c.logInfo(ip, "Starting prepare of backup", lager.Data{
		"prepareDirectory": c.prepareDirectory,
	})
	output, err := backupPrepare.CombinedOutput()
	if err != nil {
		c.logError(ip, "Preparing the backup failed", err, lager.Data{
			"output": output,
		})
		return err
	}
	c.logInfo(ip, "Successfully prepared a backup")

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
func (c *Client) writeMetadataFile(ip string, index int) error {
	src := c.originalMetadataLocation()
	dst := c.finalMetadataLocation(index)

	c.logInfo(ip, "Copying metadata file", lager.Data{
		"from": src,
		"to":   dst,
	})

	_, err := os.Create(dst)
	if err != nil {
		return err
	}

	backupMetadataMap, err := fileutils.ExtractFileFields(src)
	if err != nil {
		c.logError(ip, "Opening xtrabackup-info file failed", err)
		return err
	}

	for key, value := range c.metadataFields {
		backupMetadataMap[key] = value
	}

	for key, value := range backupMetadataMap {
		keyValLine := fmt.Sprintf("%s = %s", key, value)
		err = fileutils.WriteLineToFile(dst, keyValLine)
		if err != nil {
			c.logError(ip, "Writing metadata file failed", err)
			return err
		}
	}

	c.logInfo(ip, "Finished writing metadata file")

	return nil
}

func (c *Client) tarAndEncryptBackup(ip string, index int) error {
	c.logInfo(ip, "Starting encrypting backup")

	tarCmd := c.tarClient.Tar(c.prepareDirectory)

	encryptedFileWriter, err := os.Create(c.encryptedBackupLocation(index))
	if err != nil {
		c.logError(ip, "Error creating encrypted backup file", err)
		return err
	}
	defer encryptedFileWriter.Close()

	stdoutPipe, err := tarCmd.StdoutPipe()
	if err != nil {
		c.logError(ip, "Error attaching stdout to encryption", err)
		return err
	}

	if err := tarCmd.Start(); err != nil {
		c.logError(ip, "Error starting tar command", err)
		return err
	}

	if err := c.encryptor.Encrypt(stdoutPipe, encryptedFileWriter); err != nil {
		c.logError(ip, "Error while encrypting backup file", err)
		return err
	}

	if err := tarCmd.Wait(); err != nil {
		c.logError(ip, "Error while executing tar command", err)
		return err
	}

	c.logInfo(ip, "Successfully encrypted backup")
	return nil
}

func (c *Client) cleanDownloadDirectory(ip string) error {
	err := os.RemoveAll(c.downloadDirectory)
	if err != nil {
		c.logError(ip, fmt.Sprintf("Failed to remove %s", c.downloadDirectory), err)
		return err
	}

	c.logDebug(ip, "Cleaned download directory")
	return nil
}

func (c *Client) cleanPrepareDirectory(ip string) error {
	err := os.RemoveAll(c.prepareDirectory)
	if err != nil {
		c.logError(ip, fmt.Sprintf("Failed to remove %s", c.prepareDirectory), err)
		return err
	}

	c.logDebug(ip, "Cleaned prepare directory")
	return nil
}

func (c *Client) cleanEncryptDirectory(ip string) error {
	err := os.RemoveAll(c.encryptDirectory)
	if err != nil {
		c.logError(ip, fmt.Sprintf("Failed to remove %s", c.encryptDirectory), err)
		return err
	}

	c.logDebug(ip, "Cleaned encrypt directory")
	return nil
}

func (c *Client) cleanTmpDirectories() error {
	c.logger.Debug("Cleaning tmp directory", lager.Data{
		"tmpDirectory": c.config.TmpDir,
	})

	tmpDirs, err := filepath.Glob(filepath.Join(c.config.TmpDir, "mysql-backup*"))

	if err != nil {
		return err
	}

	for _, dir := range tmpDirs {
		err = os.RemoveAll(dir)
		if err != nil {
			c.logger.Error(fmt.Sprintf("Failed to remove tmp directory %s", dir), err)
			return err
		}
	}

	c.logger.Debug("Cleaned tmp directory")

	return nil
}

func (c *Client) cleanDirectories(ip string) error {
	c.logDebug(ip, "Cleaning directories", lager.Data{
		"downloadDirectory": c.downloadDirectory,
		"prepareDirectory":  c.prepareDirectory,
		"encryptDirectory":  c.encryptDirectory,
	})

	//continue execution even if cleanup fails
	_ = c.cleanDownloadDirectory(ip)
	_ = c.cleanPrepareDirectory(ip)
	_ = c.cleanEncryptDirectory(ip)
	return nil
}

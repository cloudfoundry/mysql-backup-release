package client

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"time"

	"code.cloudfoundry.org/lager"

	"github.com/cloudfoundry/streaming-mysql-backup-client/config"
	"github.com/cloudfoundry/streaming-mysql-backup-client/cryptkeeper"
	"github.com/cloudfoundry/streaming-mysql-backup-client/download"
	"github.com/cloudfoundry/streaming-mysql-backup-client/fileutils"
	"github.com/cloudfoundry/streaming-mysql-backup-client/tarpit"
	"github.com/cloudfoundry/streaming-mysql-backup-client/xbstream"
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

func (c *Client) artifactName(uuid string) string {
	return fmt.Sprintf("mysql-backup-%d-%s", c.version, uuid)
}

func (c *Client) downloadedBackupLocation() string {
	return path.Join(c.downloadDirectory, "unprepared-backup.tar")
}

func (c *Client) preparedBackupLocation() string {
	return path.Join(c.encryptDirectory, "prepared-backup.tar")
}

func (c *Client) encryptedBackupLocation(uuid string) string {
	return path.Join(c.config.OutputDir, fmt.Sprintf("%s.tar.gpg", c.artifactName(uuid)))
}

func (c *Client) originalMetadataLocation() string {
	return path.Join(c.prepareDirectory, "xtrabackup_info")
}

func (c *Client) finalMetadataLocation(uuid string) string {
	return path.Join(c.config.OutputDir, fmt.Sprintf("%s.txt", c.artifactName(uuid)))
}

func (c *Client) Execute() error {
	var allErrors MultiError
	var instances []config.Instance

	err := c.cleanTmpDirectories()
	if err != nil {
		return err
	}

	if c.config.BackupAllMasters {
		instances = c.config.Instances
	} else if c.config.BackupFromInactiveNode {
		var largestIndexHealthy int
		var largestIndexHealthyInstance config.Instance

		for _, instance := range c.config.Instances {
			wsrepIndex, err := c.galeraAgentCaller.WsrepLocalIndex(instance.Address)
			if err != nil {
				c.logger.Error("Fetching node status from galera agent failed", err, lager.Data{
					"ip": instance.Address,
				})
			}
			if wsrepIndex >= largestIndexHealthy {
				largestIndexHealthy = wsrepIndex
				largestIndexHealthyInstance = instance
			}
		}

		if largestIndexHealthyInstance.Address == "" {
			return errors.New("No healthy nodes found")
		}

		instances = []config.Instance{largestIndexHealthyInstance}
	} else {
		instances = []config.Instance{c.config.Instances[len(c.config.Instances)-1]}
	}

	for _, instance := range instances {
		c.version = time.Now().Unix()

		c.logger = c.config.Logger.Session("backup-"+instance.Address, lager.Data{
			"ip": instance.Address,
		})

		err := c.BackupNode(instance)
		if err != nil {
			allErrors = append(allErrors, err)
		}
		c.cleanDirectories() //ensure directories are cleaned on error
	}

	if len(allErrors) == len(instances) {
		return allErrors
	}
	return nil
}

func (c *Client) BackupNode(instance config.Instance) error {
	var err error
	err = c.createDirectories()
	if err != nil {
		return err
	}
	err = c.downloadAndUnpackBackup(instance.Address)
	if err != nil {
		return err
	}
	err = c.prepareBackup()
	if err != nil {
		return err
	}
	err = c.writeMetadataFile(instance.UUID)
	if err != nil {
		return err
	}
	err = c.tarAndEncryptBackup(instance.UUID)
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) createDirectories() error {
	c.logger.Debug("Creating directories")

	var err error
	c.downloadDirectory, err = ioutil.TempDir(c.config.TmpDir, "mysql-backup-downloads")
	if err != nil {
		c.logger.Error("Error creating temporary directory 'mysql-backup-downloads'", err)
		return err
	}

	c.prepareDirectory, err = ioutil.TempDir(c.config.TmpDir, "mysql-backup-prepare")
	if err != nil {
		c.logger.Error("Error creating temporary directory 'mysql-backup-prepare'", err)
		return err
	}

	c.encryptDirectory, err = ioutil.TempDir(c.config.TmpDir, "mysql-backup-encrypt")
	if err != nil {
		c.logger.Error("Error creating temporary directory 'mysql-backup-encrypt'", err)
		return err
	}

	c.logger.Debug("Created directories", lager.Data{
		"downloadDirectory": c.downloadDirectory,
		"prepareDirectory":  c.prepareDirectory,
		"encryptDirectory":  c.encryptDirectory,
	})

	return nil
}

func (c *Client) downloadAndUnpackBackup(ip string) error {
	c.logger.Info("Starting download of backup", lager.Data{
		"backup-prepare-path": c.prepareDirectory,
	})

	url := fmt.Sprintf("https://%s:%d/backup?format=xbstream", ip, c.config.BackupServerPort)
	err := c.downloader.DownloadBackup(url, xbstream.NewUnpacker(c.prepareDirectory))
	if err != nil {
		c.logger.Error("DownloadBackup failed", err)
		return err
	}

	c.logger.Info("Finished downloading backup", lager.Data{
		"backup-prepare-path": c.prepareDirectory,
	})

	return nil
}

func (c *Client) prepareBackup() error {
	backupPrepare := c.backupPreparer.Command(c.prepareDirectory)
	c.logger.Debug("Backup prepare command", lager.Data{
		"command": backupPrepare,
		"args":    backupPrepare.Args,
	})

	c.logger.Info("Starting prepare of backup", lager.Data{
		"prepareDirectory": c.prepareDirectory,
	})
	output, err := backupPrepare.CombinedOutput()
	if err != nil {
		c.logger.Error("Preparing the backup failed", err, lager.Data{
			"output": output,
		})
		return err
	}
	c.logger.Info("Successfully prepared a backup")

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
func (c *Client) writeMetadataFile(uuid string) error {
	src := c.originalMetadataLocation()
	dst := c.finalMetadataLocation(uuid)

	c.logger.Info("Copying metadata file", lager.Data{
		"from": src,
		"to":   dst,
	})

	_, err := os.Create(dst)
	if err != nil {
		return err
	}

	backupMetadataMap, err := fileutils.ExtractFileFields(src)
	if err != nil {
		c.logger.Error("Opening xtrabackup-info file failed", err)
		return err
	}

	for key, value := range c.metadataFields {
		backupMetadataMap[key] = value
	}

	for key, value := range backupMetadataMap {
		keyValLine := fmt.Sprintf("%s = %s", key, value)
		err = fileutils.WriteLineToFile(dst, keyValLine)
		if err != nil {
			c.logger.Error("Writing metadata file failed", err)
			return err
		}
	}

	c.logger.Info("Finished writing metadata file")

	return nil
}

func (c *Client) tarAndEncryptBackup(uuid string) error {
	c.logger.Info("Starting encrypting backup")

	tarCmd := c.tarClient.Tar(c.prepareDirectory)

	encryptedFileWriter, err := os.Create(c.encryptedBackupLocation(uuid))
	if err != nil {
		c.logger.Error("Error creating encrypted backup file", err)
		return err
	}
	defer encryptedFileWriter.Close()

	stdoutPipe, err := tarCmd.StdoutPipe()
	if err != nil {
		c.logger.Error("Error attaching stdout to encryption", err)
		return err
	}

	if err := tarCmd.Start(); err != nil {
		c.logger.Error("Error starting tar command", err)
		return err
	}

	if err := c.encryptor.Encrypt(stdoutPipe, encryptedFileWriter); err != nil {
		c.logger.Error("Error while encrypting backup file", err)
		return err
	}

	if err := tarCmd.Wait(); err != nil {
		c.logger.Error("Error while executing tar command", err)
		return err
	}

	c.logger.Info("Successfully encrypted backup")
	return nil
}

func (c *Client) cleanDownloadDirectory() error {
	err := os.RemoveAll(c.downloadDirectory)
	if err != nil {
		c.logger.Error(fmt.Sprintf("Failed to remove %s", c.downloadDirectory), err)
		return err
	}

	c.logger.Debug("Cleaned download directory")
	return nil
}

func (c *Client) cleanPrepareDirectory() error {
	err := os.RemoveAll(c.prepareDirectory)
	if err != nil {
		c.logger.Error(fmt.Sprintf("Failed to remove %s", c.prepareDirectory), err)
		return err
	}

	c.logger.Debug("Cleaned prepare directory")
	return nil
}

func (c *Client) cleanEncryptDirectory() error {
	err := os.RemoveAll(c.encryptDirectory)
	if err != nil {
		c.logger.Error(fmt.Sprintf("Failed to remove %s", c.encryptDirectory), err)
		return err
	}

	c.logger.Debug("Cleaned encrypt directory")
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

func (c *Client) cleanDirectories() error {
	c.logger.Debug("Cleaning directories", lager.Data{
		"downloadDirectory": c.downloadDirectory,
		"prepareDirectory":  c.prepareDirectory,
		"encryptDirectory":  c.encryptDirectory,
	})

	//continue execution even if cleanup fails
	_ = c.cleanDownloadDirectory()
	_ = c.cleanPrepareDirectory()
	_ = c.cleanEncryptDirectory()
	return nil
}

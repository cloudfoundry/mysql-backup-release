package client_test

import (
	"fmt"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"streaming-mysql-backup-client/config"
	"streaming-mysql-backup-client/download"

	"io/ioutil"
	"os"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	"errors"
	"streaming-mysql-backup-client/client"
	"streaming-mysql-backup-client/client/clientfakes"
	"streaming-mysql-backup-client/tarpit"
)

var _ = Describe("Streaming MySQL Backup Client", func() {
	var (
		outputDirectory    string
		backupClient       *client.Client
		rootConfig         *config.Config
		fakeDownloader     *clientfakes.FakeDownloader
		fakeBackupPreparer *clientfakes.FakeBackupPreparer
		fakeGaleraAgent    *clientfakes.FakeGaleraAgentCallerInterface
		tarClient          *tarpit.TarClient
		logger             *lagertest.TestLogger
		backupFileGlob     = `mysql-backup-*.tar.gpg`
		backupMetadataGlob = `mysql-backup-*.txt`
	)

	BeforeEach(func() {
		var err error
		outputDirectory, err = ioutil.TempDir(os.TempDir(), "backup-download-test")
		Expect(err).ToNot(HaveOccurred())

		logger = lagertest.NewTestLogger("backup-download-test")

		rootConfig = &config.Config{
			Ips:              []string{"node1"},
			BackupServerPort: 1234,
			BackupAllMasters: false,
			TmpDir:           outputDirectory,
			OutputDir:        outputDirectory,
			Logger:           logger,
			SymmetricKey:     "hello",
			MetadataFields: map[string]string{
				"compressed": "Y",
				"encrypted":  "Y",
			},
		}

		tarClient = tarpit.NewSystemTarClient()

		fakeBackupPreparer = &clientfakes.FakeBackupPreparer{}
		fakeBackupPreparer.CommandReturns(exec.Command("true"))

		fakeDownloader = &clientfakes.FakeDownloader{}

		fakeDownloader.DownloadBackupStub = func(url string, streamedWriter download.StreamedWriter) error {
			file, err := os.Open("fixtures/newtar.tar")
			Expect(err).ToNot(HaveOccurred())

			return streamedWriter.WriteStream(file)
		}

		fakeGaleraAgent = &clientfakes.FakeGaleraAgentCallerInterface{}
	})

	JustBeforeEach(func() {
		backupClient = client.NewClient(*rootConfig, tarClient, fakeBackupPreparer, fakeDownloader, fakeGaleraAgent)
	})

	AfterEach(func() {
		os.RemoveAll(outputDirectory)
	})

	It("Downloaded a file", func() {
		expectFileToNotExist(filepath.Join(outputDirectory, backupFileGlob))
		expectFileToNotExist(filepath.Join(outputDirectory, backupMetadataGlob))

		Expect(backupClient.Execute()).To(Succeed())

		expectFileToExist(filepath.Join(outputDirectory, backupFileGlob))
		expectFileToExist(filepath.Join(outputDirectory, backupMetadataGlob))
	})

	It("Filled metadata file", func() {
		Expect(backupClient.Execute()).To(Succeed())
		expectFileToExist(filepath.Join(outputDirectory, backupMetadataGlob))
		files, _ := filepath.Glob(outputDirectory + "/" + backupMetadataGlob)
		data, err := ioutil.ReadFile(files[0])
		Expect(err).ToNot(HaveOccurred())

		backupMetadataStr := string(data)

		Expect(backupMetadataStr).To(ContainSubstring("uuid ="))
		Expect(backupMetadataStr).To(ContainSubstring("name ="))
		Expect(backupMetadataStr).To(ContainSubstring("tool_name ="))
		Expect(backupMetadataStr).To(ContainSubstring("tool_command ="))
		Expect(backupMetadataStr).To(ContainSubstring("tool_version ="))
		Expect(backupMetadataStr).To(ContainSubstring("ibbackup_version ="))
		Expect(backupMetadataStr).To(ContainSubstring("server_version ="))
		Expect(backupMetadataStr).To(ContainSubstring("start_time ="))
		Expect(backupMetadataStr).To(ContainSubstring("end_time ="))
		Expect(backupMetadataStr).To(ContainSubstring("compressed ="))
		Expect(backupMetadataStr).To(ContainSubstring("encrypted ="))
	})

	It("Sets values for keys in metadata file based on MetadataFields", func() {
		Expect(backupClient.Execute()).To(Succeed())
		expectFileToExist(filepath.Join(outputDirectory, backupMetadataGlob))
		files, _ := filepath.Glob(outputDirectory + "/" + backupMetadataGlob)
		data, err := ioutil.ReadFile(files[0])
		Expect(err).ToNot(HaveOccurred())
		backupMetadataStr := string(data)

		for key, val := range rootConfig.MetadataFields {
			Expect(backupMetadataStr).To(ContainSubstring(fmt.Sprintf("%s = %s", key, val)))
		}
	})

	Context("When there are multiple URLs", func() {
		BeforeEach(func() {
			rootConfig.Ips = []string{"node1", "node2", "node3"}
		})

		Context("When BackupAllMasters is true", func() {
			BeforeEach(func() {
				rootConfig.BackupAllMasters = true
			})

			Context("When successful", func() {
				It("Creates a backup for each URL", func() {
					fakeBackupPreparer.CommandReturnsOnCall(0, exec.Command("true"))
					fakeBackupPreparer.CommandReturnsOnCall(1, exec.Command("true"))
					fakeBackupPreparer.CommandReturnsOnCall(2, exec.Command("true"))

					Expect(backupClient.Execute()).To(Succeed())

					matches, err := filepath.Glob(filepath.Join(outputDirectory, backupFileGlob))
					Expect(err).ToNot(HaveOccurred())
					Expect(matches).To(HaveLen(3))

					matches, err = filepath.Glob(filepath.Join(outputDirectory, backupMetadataGlob))
					Expect(err).ToNot(HaveOccurred())
					Expect(matches).To(HaveLen(3))
				})
			})

			Context("When unsuccessful", func() {
				Context("when all fail", func() {
					BeforeEach(func() {
						fakeBackupPreparer.CommandReturns(exec.Command("false"))
					})

					It("Returns the error", func() {
						err := backupClient.Execute()
						Expect(fakeBackupPreparer.CommandCallCount()).To(Equal(3))
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(MatchRegexp(`multiple errors:`))
						Expect(err).To(HaveLen(3))
					})

					It("Logs the failure messages", func() {
						_ = backupClient.Execute()

						Expect(logger.TestSink.Logs()).To(ContainElement(
							MatchFields(IgnoreExtras, Fields{
								"Message":  ContainSubstring("Preparing the backup failed"),
								"LogLevel": Equal(lager.ERROR),
								"Data": SatisfyAll(
									HaveKey("output"),
									HaveKeyWithValue("error", ContainSubstring("exit status 1")),
									HaveKeyWithValue("ip", Equal("node1")),
								),
							}),
						))
					})
				})

				Context("when at least one is successful", func() {
					BeforeEach(func() {
						fakeBackupPreparer.CommandReturnsOnCall(0, exec.Command("false"))
						fakeBackupPreparer.CommandReturnsOnCall(1, exec.Command("false"))
						fakeBackupPreparer.CommandReturnsOnCall(2, exec.Command("true"))

						expectFileToNotExist(filepath.Join(outputDirectory, backupFileGlob))
						expectFileToNotExist(filepath.Join(outputDirectory, backupMetadataGlob))
					})

					It("Continues to create backups and exits successfully", func() {
						Expect(backupClient.Execute()).To(Succeed())

						Expect(fakeBackupPreparer.CommandCallCount()).To(Equal(3))

						matches, err := filepath.Glob(filepath.Join(outputDirectory, backupFileGlob))
						Expect(err).ToNot(HaveOccurred())
						Expect(matches).To(HaveLen(1))

						matches, err = filepath.Glob(filepath.Join(outputDirectory, backupMetadataGlob))
						Expect(err).ToNot(HaveOccurred())
						Expect(matches).To(HaveLen(1))
					})
				})
			})
		})

		Context("When BackupFromInactiveNode is true", func() {
			BeforeEach(func() {
				rootConfig.BackupFromInactiveNode = true
				fakeGaleraAgent.WsrepLocalIndexReturnsOnCall(0, 1, nil)
				fakeGaleraAgent.WsrepLocalIndexReturnsOnCall(1, 3, nil)
				fakeGaleraAgent.WsrepLocalIndexReturnsOnCall(2, 2, nil)
			})

			Context("When successful", func() {
				It("Creates a backup only for the inactive node", func() {
					fakeBackupPreparer.CommandReturnsOnCall(0, exec.Command("true"))

					Expect(backupClient.Execute()).To(Succeed())

					matches, err := filepath.Glob(filepath.Join(outputDirectory, backupFileGlob))
					Expect(err).ToNot(HaveOccurred())
					Expect(matches).To(HaveLen(1))

					matches, err = filepath.Glob(filepath.Join(outputDirectory, backupMetadataGlob))
					Expect(err).ToNot(HaveOccurred())
					Expect(matches).To(HaveLen(1))

					Expect(fakeDownloader.Invocations()["DownloadBackup"][0][0]).To(Equal("https://node2:1234/backup"))
				})
			})

			Context("When zero nodes are healthy", func() {
				BeforeEach(func() {
					fakeGaleraAgent.WsrepLocalIndexReturnsOnCall(0, -1, errors.New("unhealthy?"))
					fakeGaleraAgent.WsrepLocalIndexReturnsOnCall(1, -1, errors.New("unhealthy?"))
					fakeGaleraAgent.WsrepLocalIndexReturnsOnCall(2, -1, errors.New("unhealthy?"))
				})

				It("Returns an error", func() {
					err := backupClient.Execute()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("No healthy nodes found"))
				})
			})

			Context("When one node is unhealthy", func() {
				BeforeEach(func() {
					fakeGaleraAgent.WsrepLocalIndexReturnsOnCall(0, 1, nil)
					fakeGaleraAgent.WsrepLocalIndexReturnsOnCall(1, 2, nil)
					fakeGaleraAgent.WsrepLocalIndexReturnsOnCall(2, -1, errors.New("unhealthy!"))
				})

				It("Logs the failure messages", func() {
					_ = backupClient.Execute()

					Expect(logger.TestSink.Logs()).To(ContainElement(
						MatchFields(IgnoreExtras, Fields{
							"Message":  ContainSubstring("Fetching node status from galera agent failed"),
							"LogLevel": Equal(lager.ERROR),
							"Data": SatisfyAll(
								HaveKeyWithValue("error", ContainSubstring("unhealthy!")),
								HaveKeyWithValue("ip", Equal("node3")),
							),
						}),
					))
				})
			})
		})

		Context("When successful", func() {
			It("Creates a backup for the last node in the array", func() {
				fakeBackupPreparer.CommandReturnsOnCall(0, exec.Command("true"))

				Expect(backupClient.Execute()).To(Succeed())

				matches, err := filepath.Glob(filepath.Join(outputDirectory, backupFileGlob))
				Expect(err).ToNot(HaveOccurred())
				Expect(matches).To(HaveLen(1))

				matches, err = filepath.Glob(filepath.Join(outputDirectory, backupMetadataGlob))
				Expect(err).ToNot(HaveOccurred())
				Expect(matches).To(HaveLen(1))

				Expect(fakeDownloader.Invocations()["DownloadBackup"][0][0]).To(Equal("https://node3:1234/backup"))
			})
		})

		Context("When unsuccessful", func() {
			BeforeEach(func() {
				fakeBackupPreparer.CommandReturns(exec.Command("false"))
			})

			It("Returns the error", func() {
				err := backupClient.Execute()
				Expect(fakeBackupPreparer.CommandCallCount()).To(Equal(1))
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("exit status 1"))
				Expect(err).To(HaveLen(1))
			})

			It("Logs the failure messages", func() {
				_ = backupClient.Execute()

				Expect(logger.TestSink.Logs()).To(ContainElement(
					MatchFields(IgnoreExtras, Fields{
						"Message":  ContainSubstring("Preparing the backup failed"),
						"LogLevel": Equal(lager.ERROR),
						"Data": SatisfyAll(
							HaveKey("output"),
							HaveKeyWithValue("error", ContainSubstring("exit status 1")),
							HaveKeyWithValue("ip", Equal("node3")),
						),
					}),
				))
			})
		})
	})

})

func expectFileToNotExist(glob string) {
	matches, err := filepath.Glob(glob)
	Expect(err).ToNot(HaveOccurred())
	Expect(matches).To(HaveLen(0), fmt.Sprintf("Expected no files to match glob: %s", glob))
}

func expectFileToExist(glob string) {
	matches, err := filepath.Glob(glob)
	Expect(err).ToNot(HaveOccurred())
	Expect(matches).ToNot(HaveLen(0), fmt.Sprintf("Expected at least one file to match glob: %s", glob))
}

# streaming-mysql-backup-client


## Usage as a library
```go
tlsConfig := &tls.Config{}
downloader := download.NewDownloaderFromCredentials("username", "password", tlsConfig)
untarStreamer := tarpit.NewUntarStreamer("/path/to/mysql/data")

err := downloader.DownloadBackup("http://{streaming-mysql-backup-tool}:8081/backup", untarStreamer)
if err != nil {
	log.Fatal(err)
}
```

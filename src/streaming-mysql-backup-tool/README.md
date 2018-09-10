# streaming-mysql-backup-tool
A streaming backup tool for MySQL

## Usage:
This tool is colocated on each mysql node.
It listens for an HTTP request to start a backup, and then streams the backup off the mysql node as part of the HTTP response.

## Install Dependencies
This project uses [dep](https://github.com/golang/dep) to manage its dependencies.

Install dep following instructions [here](https://github.com/golang/dep).

Run `dep ensure` to download dependencies to `vendor`, and check them in to version control.

## Run tests

Install ginkgo:
```
go get github.com/onsi/ginkgo/ginkgo
```

Run tests:
```
./bin/test
```

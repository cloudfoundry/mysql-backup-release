# streaming-mysql-backup-tool
A streaming backup tool for MySQL

## Usage:
This tool is colocated on each mysql node.
It listens for an HTTP request to start a backup, and then streams the backup off the mysql node as part of the HTTP response.

## Install Dependencies
This project uses [godep](https://github.com/tools/godep) to manage its dependencies.
All dependencies are added to `vendor` and checked into version control.

Install godep:
```
go get github.com/tools/godep
```

Then, run `godep restore` to download these dependencies.

## Run tests

Install ginkgo:
```
go get github.com/onsi/ginkgo/ginkgo
```

Run tests:
```
./bin/test
```

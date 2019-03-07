# MySQL Backup Release

## Usage

MySQL Backup Release provides an api to stream a backup to a backup prepare node.
The [service-backup-release](https://github.com/pivotal-cf/service-backup-release)
can be used to send that backup to any supported blobstore.

This release works with [cf-mysql-release](https://github.com/cloudfoundry/cf-mysql-release) or with [pxc-release](https://github.com/cloudfoundry-incubator/pxc-release).

## Components

This release provides two components that communicate with each other in order
to take a backup of a MySQL cluster.

The streaming-mysql-backup-client is co-located with the long-running service-backup
job. This job will run the client on a schedule to start the backup process.

The streaming-mysql-backup-client will then stream a backup off a MySQL VM by contacting the
streaming-mysql-backup-tool. This tool listens for an HTTP request and will stream back a
backup as part of the HTTP response.

The backup will be temporarily stored on the VM with the long-running service
backup job before being uploaded to its final destination (by another service such as the [service-backup-release](https://github.com/pivotal-cf/service-backup-release)).

## Deploying mysql-backup-release

To prepare, you will need:

* [mysql-backup-release](https://github.com/cloudfoundry-incubator/mysql-backup-release) repository
* [service-backup-release](https://github.com/pivotal-cf/service-backup-release)

#### Prepare destinations
See [documentation for service-backup-release](https://github.com/pivotal-cf/service-backup-release/blob/master/README.md) to set up your destinations.

#### Create your variables file

Create a file, `backup-variables.yml`

```yml
---
# see https://godoc.org/github.com/robfig/cron for syntax
service_backup_cron_schedule: "@hourly"        

# from the 'destinations' configuration in https://github.com/pivotal-cf/service-backup-release
service_backup_destinations:                   
- type: s3
  config:
    endpoint_url: https://s3.amazonaws.com
    bucket_name: some-bucket
    bucket_path: path/inside/of/bucket
    access_key_id: foo
    secret_access_key: bar
```

#### Upload releases

Backups uses:

* [mysql-backup-release](https://github.com/cloudfoundry-incubator/mysql-backup-release)
* [service-backup-release](https://github.com/pivotal-cf/service-backup-release)

#### Deploy

##### Deploying with pxc-release
```
$ bosh --deployment=pxc deploy pxc-release/pxc-deployment.yml \
    ...
    --ops-file=mysql-backup-release/operations/add-backup.yml \
    --ops-file=mysql-backup-release/operations/pxc.yml \
    --vars-file=backup-variables.yml
```

##### Deploying with cf-mysql-release
```
$ bosh --deployment=cf-mysql deploy cf-mysql-deployment/cf-mysql-deployment.yml \
    ...
    --ops-file=mysql-backup-release/operations/add-backup.yml \
    --vars-file=backup-variables.yml
```


## Restoring a Galera cluster from a backup artifact

After deploying mysql-backup-release, you should be generating backup artifacts
to your external file store. To restore a database cluster from one of these
artifacts, follow the directions below.  For the following directions the
`${data_directory}` will be `/var/vcap/store/pxc-mysql` for pxc-release and
`/var/vcap/store/mysql` for cf-mysql-release.

1. Redeploy with the following operations file to scale the cluster down to one mysql node:

  ```yaml
    - type: replace
      path: /instance_groups/name=mysql/instances
      value: 1

    - type: remove
      path: /instance_groups/name=arbitrator?
  ```

1. Prepare the first node for restoration
	1. ssh onto MySQL node
	1. `sudo su`
	1. `monit stop all` (will restart once data has been restored)
	1. `watch monit summary` until all jobs are listed as 'not monitored'
	1. `rm -rf ${data_directory}` (delete the existing mysql data which is stored on disk)
	1. `mkdir -p ${data_directory}` (recreate the data directory)
1. Restore the backup
	1. Move the encrypted backup (named e.g. `mysql-backup.tar.gpg`) to the node (e.g. via `bosh scp`)
	1. Decrypt the backup with your encryption passphrase: `gpg --compress-algo zip --cipher-algo AES256 --output mysql-backup.tar --decrypt mysql-backup.tar.gpg`
	1. `tar -xvf mysql-backup.tar --directory=${data_directory}` (untar the backup artifact into the data directory of MySQL)
	1. `chown -R vcap:vcap ${data_directory}` (MySQL process expects data directory to be owned by a particular user)
	1. `monit start all`
	1. `watch monit summary` until all jobs are listed as 'running'
	1. Exit out of the MySQL node
1. Increase the size of the cluster back to its original
	1. Re-deploy to scale back up to three nodes.

## Development

- Clone the repo:
  ```bash
  git clone git@github.com:cloudfoundry-incubator/mysql-backup-release.git
  ```

- Ensure `direnv` is installed
 - E.g. via brew:
  ```bash
  brew install direnv
  ```
The BOSH release directory serves as the `GOPATH`.
The `$GOPATH` is automatically configured when you `cd` into the release directory via `direnv` and the `.envrc` file.

- Sync the submodules.
  ```bash
  cd /path/to/bosh/release
  ./scripts/update
  ```

- Run unit tests for all submodules:
  ```bash
  ./scripts/test-unit
  ```

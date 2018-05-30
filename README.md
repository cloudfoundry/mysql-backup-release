# MySQL Backup Release

## Usage

This is a release which works with [cf-mysql-release](https://github.com/cloudfoundry/cf-mysql-release)
and [service-backup-release](https://github.com/pivotal-cf-experimental/service-backup-release). P-mysql
uses all three to perform automated backups of a MySQL cluster.

## Components

This release provides two components that communicate with each other in order
to take a backup of a MySQL cluster.

The streaming-mysql-backup-client is co-located with the long-running service-backup
job. This job will run the client on a schedule to start the backup process.

The streaming-mysql-backup-client will then stream a backup off a MySQL VM by contacting the
streaming-mysql-backup-tool. This tool listens for an HTTP request and will stream back a
backup as part of the HTTP response.

The backup will be temporarily stored on the VM with the long-running service
backup job before being uploaded to its final destination.

## Deploying mysql-backup-release

To prepare, you will need:

* [mysql-backup-release](https://github.com/pivotal-cf-experimental/mysql-backup-release) repository
* [service-backup-release](https://github.com/pivotal-cf-experimental/service-backup-release)

#### Prepare destinations
See [documentation for service-backup-release](https://docs.pivotal.io/service-backup/) to set up your destinations.

#### Create your variables file

Create a file, `backup-variables.yml`

```yml
---
mysql_backup:
  symmetric_key: secret-password
  endpoint_credentials:
    username: username
    password: password

# see https://godoc.org/github.com/robfig/cron for syntax
service_backup_cron_schedule: "@hourly"        

# from the 'destinations' configuration in https://docs.pivotal.io/service-backup/
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

* [mysql-backup-release](https://github.com/pivotal-cf-experimental/mysql-backup-release)
* [service-backup-release](https://docs.pivotal.io/service-backup/)

#### Deploy

Follow the [new deployment procedure for p-mysql-deployment](https://github.com/cloudfoundry/p-mysql-deployment) and provide:

* an extra ops-file: [add-backup.yml](https://github.com/pivotal-cf-experimental/p-mysql-deployment/operations/add-backup.yml)
* an extra var-file: `backup-variables.yml` from above

## Restoring a cf-mysql cluster from a backup artifact

After deploying mysql-backup-release, you should be generating backup artifacts to your external file store. To restore a database cluster from one of these artifacts, follow the directions below:

1. Redeploy with the following operations file to scale the cluster down to one mysql node:

  ```yaml
    - type: replace
      path: /instance_groups/name=mysql/instances
      value: 1

    - type: replace
      path: /instance_groups/name=arbitrator/instances
      value: 0

    - type: replace
      path: /instance_groups/name=backup-prepare/instances
      value: 0
  ```

1. Prepare the first node for restoration
	1. ssh onto MySQL node
	1. `sudo su`
	1. `monit stop all` (will restart once data has been restored)
	1. `watch monit summary` until all jobs are listed as 'not monitored'
	1. `rm -rf /var/vcap/store/mysql/*` (delete the existing mysql data which is stored on disk)
1. Restore the backup
	1. Move the encrypted backup (named e.g. `mysql-backup.tar.gpg`) to the node (e.g. via `scp`)
	1. Decrypt the backup with your encryption passphrase: `gpg --compress-algo zip --cipher-algo AES256 --output mysql-backup.tar --decrypt mysql-backup.tar.gpg`
	1. `tar xvf mysql-backup.tar --directory=/var/vcap/store/mysql` (untar the backup artifact into the data directory of MySQL)
	1. `chown -R vcap:vcap /var/vcap/store/mysql` (MySQL process expects data directory to be owned by a particular user)
	1. `monit start all`
	1. `watch monit summary` until all jobs are listed as 'running'
	1. Exit out of the MySQL node
1. Increase the size of the cluster back to its original
	1. Re-deploy without the extra `scale-down-cluster.yml` ops-file

## Development

- Clone the repo:
  ```bash
  git clone git@github.com:pivotal-cf-experimental/mysql-backup-release.git
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

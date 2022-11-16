#!/usr/bin/env bash

set -o errexit -o nounset

: "${MYSQL_ENGINE:="pxc-8.0"}"

export MYSQL_ENGINE

ginkgo "$@"

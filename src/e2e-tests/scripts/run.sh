#!/usr/bin/env bash

set -o errexit -o nounset

test_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." &>/dev/null && pwd)

: "${MYSQL_ENGINE:="pxc-8.0"}"

export MYSQL_ENGINE

cd "${test_dir}"
go run github.com/onsi/ginkgo/v2/ginkgo "$@"
cd -

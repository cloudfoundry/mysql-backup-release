#!/bin/bash

set -euo pipefail

GNUPGHOME=$(mktemp -d) \
    gpg --batch --passphrase="${GPG_PASSPHRASE}" --decrypt < "${ARTIFACTS_PATH}"/*.tar.gpg \
        | tar -x -C "${DATA_DIRECTORY}"
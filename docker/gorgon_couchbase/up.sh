#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

cd $(dirname "$0")
pwd

make DB_NODES=${DB_NODES:-3}

docker compose -f compose.yaml up --force-recreate $*

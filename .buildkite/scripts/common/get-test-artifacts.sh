#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

set -euo pipefail

# Required environment variables:
#   VAULT_ROOT_PATH
#   IS_SNAPSHOT_BUILD

source .env

secretFieldPrefix=""

if [[ "${IS_SNAPSHOT_BUILD:-}" != "" ]]; then

    secretFieldPrefix="dev-"

    vault read -field="dev-privkey" "${VAULT_ROOT_PATH}/license" | base64 --decode > .ci/dev-private.key

fi

vault read -field="${secretFieldPrefix:-}enterprise" "${VAULT_ROOT_PATH}/test-licenses" > .ci/test-license.json

vault read -field="${secretFieldPrefix:-}pubkey" "${VAULT_ROOT_PATH}/license" | base64 --decode > .ci/license.key

vault read -field="data" -format=json "${VAULT_ROOT_PATH}/monitoring-cluster" > .ci/monitoring-secrets.json

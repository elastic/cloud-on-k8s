#!/usr/bin/env bash
set -euo pipefail

vault read -field="data" -format=json "${VAULT_ROOT_PATH}/helm-charts-publisher" > .ci/credentials.json
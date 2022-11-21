#!/usr/bin/env bash
set -euo pipefail

if vault read -field="data" -format=json "${VAULT_ROOT_PATH}/helm-charts-publisher" > .ci/credentials.json ; then
    echo "Vault read of helm credentials succeeded"
else
    echo "Vault read of helm credentials failed"
    exit 1
fi

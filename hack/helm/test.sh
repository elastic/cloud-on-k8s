#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to test helm charts

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

check() {
    local TEST_DIR="$1"
    cd "${TEST_DIR}"

    echo "Ensuring dependencies are updated for $(basename "${TEST_DIR}") chart."
    helm dependency update . 1>/dev/null
    
    echo "Running 'helm lint' on $(basename "${TEST_DIR}") chart."
    helm lint --strict .

    docker run -ti --rm -v "$(pwd)":/apps quintush/helm-unittest:latest -3 -f 'templates/tests/*.yaml' .
    cd -
}

for i in "${SCRIPT_DIR}"/../../deploy/[a-zA-Z0-9]*; do
    if [[ ! -d "${i}" ]]; then
        continue
    fi
    if [[ -d "${i}"/templates/tests ]]; then
        check "${i}"
    fi
done

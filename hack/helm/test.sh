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

    docker run -ti --rm -v "$(pwd)":/apps quintush/helm-unittest:latest -3 -f 'templates/tests/*.yaml' .
    cd -
}

for i in $(ls "${SCRIPT_DIR}"/../../deploy/); do
    if [[ ! -d "${SCRIPT_DIR}"/../../deploy/"${i}" ]]; then
        continue
    fi
    if [[ -d "${SCRIPT_DIR}"/../../deploy/"${i}"/templates/tests ]]; then
        check "${SCRIPT_DIR}"/../../deploy/"${i}"
    fi
done

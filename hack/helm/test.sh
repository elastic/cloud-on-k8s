#!/usr/bin/env bash
#
# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.
#
# Script to test helm charts
#
set -eu

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

sed_gnu() { sed -i "$@"; }
sed_bsd() { sed -i '' "$@"; }

list_chart_dirs() {
    find "${SCRIPT_DIR}/../../deploy" -type f -name "Chart.yaml" -exec dirname "{}" \;| sort -u
}

check() {
    local TEST_DIR="$1"
    cd "${TEST_DIR}"

    echo "Ensuring dependencies are updated for $(basename "${TEST_DIR}") chart."
    helm dependency update . 1>/dev/null
    
    echo "Running 'helm lint' on $(basename "${TEST_DIR}") chart."
    if [[ -f "lint-values.yaml" ]]; then
        helm lint --strict -f lint-values.yaml .
    else
        helm lint --strict .
    fi

    if [[ -d templates/tests ]]; then
        helm unittest -3 -f 'templates/tests/*.yaml' --with-subchart=false .
    fi

    cd -
}

for i in $(list_chart_dirs); do
    check "${i}"
done

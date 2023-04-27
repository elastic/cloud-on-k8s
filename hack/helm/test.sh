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

    local SED="sed_gnu"
    if [[ "$OSTYPE" =~ "darwin" ]]; then
        SED="sed_bsd"
    fi

    # Ensure official helm repository is commented out in Chart.yaml
    "$SED" -E 's|[[:space:]]+repository: "https://helm\.elastic\.co"|    # repository: "https://helm.elastic.co"|g' Chart.yaml
    # Uncomment file:// repository stanzas to ensure local changes to repositories are used instead
    "$SED" -E 's|.*repository: "file://\.\./(eck-[a-z-]+)"$|    repository: "file://../\1"|' Chart.yaml

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

    # restore changes to Chart.yaml
    git checkout Chart.yaml

    cd -
}

for i in $(list_chart_dirs); do
    check "${i}"
done

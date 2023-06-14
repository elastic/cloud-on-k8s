#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to annotate and notify about e2e tests failures.

set -eu

WD="$(cd "$(dirname "$0")"; pwd)"
ROOT="$WD/../../.."

convert() {
    local jsonFile=${1:-"json report"}

    # temporary filter out lines containing a space in the timestamp,
    # see https://github.com/elastic/cloud-on-k8s/issues/3560.
    gotestsum \
        --junitfile "${jsonFile%.*}.xml" \
        --raw-command grep -v '"Time":"[^"]*\s[^"]*"' "$jsonFile" >/dev/null || \
    ( \
        echo "Error: failed to generate JUnit XML report for $jsonFile"
        # print the input file for further debugging
        echo " --- $jsonFile - START ---"
        cat "$jsonFile"
        echo " --- $jsonFile - END   ---"
    )
}

main() {
    cd "$ROOT/.buildkite/e2e/reporter"

    mkdir reports/
    buildkite-agent artifact download "*.json" reports/

    for f in reports/*.json; do
        convert "$f"
    done

    set +e

    go run main.go -d reports -o annotate-success  > success.md
    if [[ -s success.md ]]; then
        buildkite-agent annotate --style success --context success < success.md
    fi

    go run main.go -d reports -o annotate-failures > failures.md
    if [[ -s failures.md ]]; then
        buildkite-agent annotate --style error --context error < failures.md
    fi

    go run main.go -d reports -o notify-failures > notify.yml
    buildkite-agent pipeline upload notify.yml
}

main

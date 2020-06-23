#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

# Script to generate a NOTICE file containing licence information from dependencies.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR=${SCRIPT_DIR}/../..
TEMP_DIR=$(mktemp -d)
LICENCE_DETECTOR="go.elastic.co/go-licence-detector@v0.3.0"

trap '[[ $TEMP_DIR ]] && rm -rf "$TEMP_DIR"' EXIT

get_licence_detector() {
    (
        cd "$TEMP_DIR"
        GOBIN="$TEMP_DIR" GO111MODULE=on go get "$LICENCE_DETECTOR"
    )
}

generate_notice() {
    (
        cd "$PROJECT_DIR"
        go mod download
        go list -m -json all | "${TEMP_DIR}"/go-licence-detector \
            -depsTemplate="${SCRIPT_DIR}"/templates/dependencies.asciidoc.tmpl \
            -depsOut="${PROJECT_DIR}"/docs/reference/dependencies.asciidoc \
            -noticeTemplate="${SCRIPT_DIR}"/templates/NOTICE.txt.tmpl \
            -noticeOut="${PROJECT_DIR}"/NOTICE.txt \
            -overrides="${SCRIPT_DIR}"/overrides/overrides.json \
            -rules="${SCRIPT_DIR}"/rules.json \
            -includeIndirect
    )
}

echo "Generating notice file and dependency list"
get_licence_detector
generate_notice

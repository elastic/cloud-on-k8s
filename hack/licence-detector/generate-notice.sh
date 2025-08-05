#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to generate a NOTICE file containing licence information from dependencies.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR=${SCRIPT_DIR}/../..
TEMP_DIR=$(mktemp -d)
LICENCE_DETECTOR="go.elastic.co/go-licence-detector@v0.9.0"

# Load the version script to get the current version of the project.
# shellcheck disable=SC1091
source "${SCRIPT_DIR}"/../version.sh

trap '[[ $TEMP_DIR ]] && rm -rf "$TEMP_DIR"' EXIT

get_licence_detector() {
    GOBIN="$TEMP_DIR" go install "$LICENCE_DETECTOR"
}

generate_notice() {
    (
        version="$(get_current_version)"
        # Remove dots from the version string for compatibility with the doc web site.
        outFile="${version//./_}.md"
        echo "Generating notice file and dependency list for version: ${version}, output file: ${outFile}"
        cd "$PROJECT_DIR"
        go mod download
        go list -m -json all | "${TEMP_DIR}"/go-licence-detector \
            -depsTemplate="${SCRIPT_DIR}"/templates/dependencies.md.tmpl \
            -template-value=eckVersion="${version}" \
            -depsOut="${PROJECT_DIR}"/docs/reference/third-party-dependencies/"${outFile}" \
            -noticeTemplate="${SCRIPT_DIR}"/templates/NOTICE.txt.tmpl \
            -noticeOut="${PROJECT_DIR}"/NOTICE.txt \
            -overrides="${SCRIPT_DIR}"/overrides/overrides.json \
            -rules="${SCRIPT_DIR}"/rules.json \
            -includeIndirect
    )
}

get_licence_detector
generate_notice

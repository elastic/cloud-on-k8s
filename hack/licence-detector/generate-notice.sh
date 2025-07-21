#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to generate a NOTICE file containing licence information from dependencies.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR=${SCRIPT_DIR}/../..
TEMP_DIR=$(mktemp -d)
LICENCE_DETECTOR="go.elastic.co/go-licence-detector@template-variables"

trap '[[ $TEMP_DIR ]] && rm -rf "$TEMP_DIR"' EXIT

get_licence_detector() {
    GOBIN="$TEMP_DIR" go install "$LICENCE_DETECTOR"
}

is_version() {
  [[ "$1" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]
}

get_current_version() {
  version="$(<"${PROJECT_DIR}/VERSION")"
  if is_version "${version}"; then
    echo "${version}"
  else
    echo "main"
  fi
}

generate_notice() {
    (
        version="$(get_current_version)"
        # Remove dots from the version string for compatibility with the doc web site.
        outFile="${version//./_}.md"
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

echo "Generating notice file and dependency list"
get_licence_detector
generate_notice

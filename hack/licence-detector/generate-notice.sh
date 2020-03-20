#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

# Script to generate a NOTICE file containing licence information from dependencies.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR=${SCRIPT_DIR}/../..

build_licence_detector() {
    (
        cd "$SCRIPT_DIR"
        go build -v github.com/elastic/cloud-on-k8s/hack/licence-detector
    )
}

generate_notice() {
    (
        cd "$PROJECT_DIR"
        go mod download
        go list -m -json all | "${SCRIPT_DIR}"/licence-detector \
            -licenceData="${SCRIPT_DIR}"/licence.db \
            -depsTemplate="${SCRIPT_DIR}"/templates/dependencies.asciidoc.tmpl \
            -depsOut="${PROJECT_DIR}"/docs/reference/dependencies.asciidoc \
            -noticeTemplate="${SCRIPT_DIR}"/templates/NOTICE.txt.tmpl \
            -noticeOut="${PROJECT_DIR}"/NOTICE.txt \
            -overrides="${SCRIPT_DIR}"/overrides/overrides.json \
            -includeIndirect
    )
}

build_licence_detector
generate_notice

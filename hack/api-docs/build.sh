#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

# Script to generate API reference documentation from the source code.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="${SCRIPT_DIR}/../.."
DOCS_DIR="${SCRIPT_DIR}/../../docs"
REFDOCS_REPO="https://github.com/elastic/gen-crd-api-reference-docs.git"
REFDOCS_VER="master"

log() {
    echo ">> $1"
}

build_docs() {
    local INSTALL_DIR=$(mktemp -d)
    (
        log "Building refdoc binary..."
        cd $INSTALL_DIR
        git clone -q --depth 1 --branch $REFDOCS_VER --single-branch $REFDOCS_REPO
        cd gen-crd-api-reference-docs
        go mod edit --replace=github.com/elastic/cloud-on-k8s@latest=$REPO_ROOT
        go build
    )

    local REFDOCS_BIN=${INSTALL_DIR}/gen-crd-api-reference-docs/gen-crd-api-reference-docs
    local TEMP_OUT_FILE=$(mktemp)
    (
        log "Generating API reference documentation..."
        cd $REPO_ROOT
        $REFDOCS_BIN -api-dir=./pkg/apis \
            -template-dir="${SCRIPT_DIR}/templates" \
            -out-file=$TEMP_OUT_FILE \
            -config="${SCRIPT_DIR}/config.json" \
            -logtostderr=true \
            -stderrthreshold=3
        mv $TEMP_OUT_FILE "${DOCS_DIR}/api-docs.asciidoc"
        log "API reference documentation generated successfully."
    )

    rm -rf $INSTALL_DIR || log "Failed to clean-up $INSTALL_DIR"
}

build_docs

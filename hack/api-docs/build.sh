#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

# Script to generate API reference documentation from the source code.

set -euo pipefail

SCRATCH_DIR=$(mktemp -d)

cleanup() {
    rm -rf $SCRATCH_DIR || echo "Failed to clean-up $SCRATCH_DIR"
}

trap cleanup EXIT


SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="${SCRIPT_DIR}/../.."
DOCS_DIR="${SCRIPT_DIR}/../../docs"
REFDOCS_REPO="https://github.com/elastic/gen-crd-api-reference-docs.git"
REFDOCS_VER="master"

log() {
    echo ">> $1"
}

build_docs() {
    local INSTALL_DIR=$SCRATCH_DIR
    (
        log "Building refdoc binary..."
        cd $INSTALL_DIR
        git clone -q --depth 1 --branch $REFDOCS_VER --single-branch $REFDOCS_REPO
        cd gen-crd-api-reference-docs
        go mod edit --replace=github.com/elastic/cloud-on-k8s@latest=$REPO_ROOT
        go build
    )

    local REFDOCS_BIN=${INSTALL_DIR}/gen-crd-api-reference-docs/gen-crd-api-reference-docs
    local TEMP_OUT_FILE="${SCRATCH_DIR}/api-docs.asciidoc"
    (
        log "Generating API reference documentation..."
        cd $REPO_ROOT
        $REFDOCS_BIN -api-dir=./pkg/apis \
            -template-dir="${SCRIPT_DIR}/templates" \
            -out-file=$TEMP_OUT_FILE \
            -config="${SCRIPT_DIR}/config.json" \
            -log_dir=$SCRATCH_DIR \
            -alsologtostderr=false \
            -logtostderr=false \
            -stderrthreshold=ERROR
        cp $TEMP_OUT_FILE "${DOCS_DIR}/api-docs.asciidoc"
        log "API reference documentation generated successfully."
    )
}

build_docs
cleanup

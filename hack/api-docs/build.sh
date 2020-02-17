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
REFDOCS_REPO="github.com/elastic/crd-ref-docs"
REFDOCS_VER="v0.0.3"

log() {
    echo ">> $1"
}

build_docs() {
    local BIN_DIR=${SCRATCH_DIR}/bin
    (
        log "Installing crd-ref-docs $REFDOCS_VER"
        cd $SCRATCH_DIR
        mkdir -p $BIN_DIR
        go mod init github.com/elastic/cloud-on-k8s-docs && GOBIN=$BIN_DIR go get -u "${REFDOCS_REPO}@${REFDOCS_VER}"

        log "Generating API reference documentation..."
        ${BIN_DIR}/crd-ref-docs --source-path=${REPO_ROOT}/pkg/apis \
            --config=${SCRIPT_DIR}/config.yaml \
            --renderer=asciidoctor \
            --templates-dir=${SCRIPT_DIR}/templates \
            --output-path=${DOCS_DIR}/api-docs.asciidoc
    )
}

build_docs
cleanup

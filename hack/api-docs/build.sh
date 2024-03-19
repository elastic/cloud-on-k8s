#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to generate API reference documentation from the source code.
# To test with a different version of crd-ref-docs while retaining the binary, invoke the script as follows:
# SCRATCH_DIR=/tmp/crd-ref-docs-tmp CLEANUP=false REFDOCS_VER=e36d311 ./build.sh

set -euo pipefail

SCRATCH_DIR="${SCRATCH_DIR:-$(mktemp -d -t crd-ref-docs-XXXXX)}"
CLEANUP="${CLEANUP:-true}"

cleanup() {
    if [[ $CLEANUP == "true" ]]; then
        echo "Removing $SCRATCH_DIR"
        rm -rf "$SCRATCH_DIR" || echo "Failed to remove $SCRATCH_DIR"
    fi
}

build_docs() {
    local SCRIPT_DIR
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    local REPO_ROOT="${SCRIPT_DIR}/../.."
    local DOCS_DIR="${SCRIPT_DIR}/../../docs"
    local REFDOCS_REPO="${REFDOCS_REPO:-github.com/elastic/crd-ref-docs}"
    local REFDOCS_VER="${REFDOCS_VER:-v0.0.12}"
    local BIN_DIR=${SCRATCH_DIR}/bin

    (
        echo "Installing crd-ref-docs $REFDOCS_VER to $BIN_DIR"
        mkdir -p "$BIN_DIR"
        GOBIN=$BIN_DIR go install "${REFDOCS_REPO}@${REFDOCS_VER}"

        echo "Generating API reference documentation"
        "${BIN_DIR}"/crd-ref-docs --source-path="${REPO_ROOT}"/pkg/apis \
            --config="${SCRIPT_DIR}"/config.yaml \
            --renderer=asciidoctor \
            --templates-dir="${SCRIPT_DIR}"/templates \
            --output-path="${DOCS_DIR}"/reference/api-docs.asciidoc
    )
}

trap cleanup EXIT
build_docs

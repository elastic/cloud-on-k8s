#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

# Script to generate API reference documentation from the source code.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DOCS_DIR="${SCRIPT_DIR}/../../docs"
REFDOCS_PKG="github.com/elastic/gen-crd-api-reference-docs"
REFDOCS_VER="master"
REFDOCS_BIN="$(go env GOPATH)/bin/$(basename $REFDOCS_PKG)"

install_refdocs() {
    local INSTALL_DIR=$(mktemp -d)
    (
        cd $INSTALL_DIR
        GO111MODULE=on go mod init github.com/elastic/eck-refdocs
        GO111MODULE=on go get -u "${REFDOCS_PKG}@${REFDOCS_VER}"
    )
}

build_docs() {
    local TEMP_OUT_FILE=$(mktemp)
    $REFDOCS_BIN -api-dir=github.com/elastic/cloud-on-k8s/pkg/apis \
        -template-dir="${SCRIPT_DIR}/templates" \
        -out-file=$TEMP_OUT_FILE \
        -config="${SCRIPT_DIR}/config.json"

    mv $TEMP_OUT_FILE "${DOCS_DIR}/api-docs.asciidoc"
}

if [[ ! -x "$REFDOCS_BIN" ]]; then
    install_refdocs
fi
build_docs

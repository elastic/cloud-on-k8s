#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

# Script to generate API reference documentation from the source code.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DOCS_DIR="${SCRIPT_DIR}/../../../docs"
# TODO: revert to main repo if https://github.com/ahmetb/gen-crd-api-reference-docs/pull/11 gets merged
#REFDOCS_PKG="github.com/ahmetb/gen-crd-api-reference-docs"
#REFDOCS_VER="v0.1.5"
REFDOCS_PKG="github.com/charith-elastic/gen-crd-api-reference-docs"
REFDOCS_VER="asciidoc-fix"
REFDOCS_BIN="$(go env GOPATH)/bin/$(basename $REFDOCS_PKG)"

install_refdocs() {
    local INSTALL_DIR=$(mktemp -d)
    (
        cd $INSTALL_DIR
        go mod init github.com/elastic/eck-refdocs
        go get -u "${REFDOCS_PKG}@${REFDOCS_VER}"
    )
}

build_docs() {
    local TEMP_OUT_FILE=$(mktemp)
    $REFDOCS_BIN -api-dir=github.com/elastic/cloud-on-k8s/operators/pkg/apis \
        -template-dir="${SCRIPT_DIR}/templates" \
        -out-file=$TEMP_OUT_FILE \
        -config="${SCRIPT_DIR}/config.json"

    sed -e 's|\(<br/>\)\+|\n|g' $TEMP_OUT_FILE > "${DOCS_DIR}/api-docs.asciidoc"
}

if [[ ! -x "$REFDOCS_BIN" ]]; then
    install_refdocs
fi
build_docs

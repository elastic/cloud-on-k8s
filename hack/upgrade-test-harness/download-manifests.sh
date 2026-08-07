#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [[ $# -ne 1 ]]; then
    echo "Usage: $0 <version> (e.g. 3.5.0)"
    exit 1
fi

VERSION="$1"
FOLDER="v$(echo "${VERSION}" | tr -d '.')"
BASE_URL="https://download.elastic.co/downloads/eck/${VERSION}"

mkdir -p "${SCRIPT_DIR}/testdata/${FOLDER}"
curl -fsSL "${BASE_URL}/crds.yaml" -o "${SCRIPT_DIR}/testdata/${FOLDER}/crds.yaml"
curl -fsSL "${BASE_URL}/operator.yaml" -o "${SCRIPT_DIR}/testdata/${FOLDER}/install.yaml"

echo "Downloaded ECK ${VERSION} into ${SCRIPT_DIR}/testdata/${FOLDER}/"

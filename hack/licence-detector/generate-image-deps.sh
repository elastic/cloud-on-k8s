#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

# Script to generate licence information for container image dependencies.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR=${SCRIPT_DIR}/../..

IMAGE_NAME=${IMAGE:-docker.elastic.co/eck/eck-operator}
IMAGE_TAG=${IMAGE_TAG:-1.0.1}
OUT_FILE=${OUT_FILE:-"${PROJECT_DIR}/docs/reference/container-image-dependencies.csv"}
SCRATCH_DIR=${SCRATCH_DIR:-$(mktemp -d)}

install_tern() {
    (
        cd "$SCRATCH_DIR"
        python3 -m venv ternenv
        ternenv/bin/pip install tern
    )
}

generate_csv() {
    "${SCRATCH_DIR}"/ternenv/bin/tern -l report -f json -i "${IMAGE_NAME}:${IMAGE_TAG}" | \
        jq -r '.images[].image.layers[].packages |= sort_by(.name) | .images[].image.layers[].packages[] | [.name, .version, .pkg_license, .proj_url] | @csv' > "${OUT_FILE}"
}


if [[ ! -f "${SCRATCH_DIR}/ternenv/bin/tern" ]]; then
    install_tern
fi
generate_csv

#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

# Script to extract ECK configuration map contents from all-in-one.yaml

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT_FILE="${SCRIPT_DIR}/../../config/eck.yaml"

(
    cd "$SCRIPT_DIR"
    "${SCRIPT_DIR}"/../manifest-gen/manifest-gen.sh -g --set=installCRDs=false --set=webhook.enabled=false | go run main.go > "$OUT_FILE"
)

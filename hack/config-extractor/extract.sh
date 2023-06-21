#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to extract ECK configuration map contents from all-in-one.yaml

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT_FILE="${SCRIPT_DIR}/../../config/eck.yaml"

(
    cd "$SCRIPT_DIR"
    "${SCRIPT_DIR}"/../manifest-gen/manifest-gen.sh -g \
    --set=installCRDs=false \
    --set=webhook.enabled=false \
    --set=telemetry.distributionChannel=image \
    | go run main.go > "$OUT_FILE"
    echo >> "$OUT_FILE" # empty line at EOF
)

#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

# Script to generate an ECK installation manifest

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KUBE_VERSION="${KUBE_VERSION:-1.18.0}"


TEMP_DIR="${TEMP_DIR:-$(mktemp -d -t manifest-gen-XXXXX)}"
trap 'rm -rf "$TEMP_DIR"' EXIT

MG="${SCRIPT_DIR}/manifest-gen.sh -g"

check() {
    local TEST_NAME="$1"

    echo "[TEST] $TEST_NAME"
    docker run -it -v "${TEMP_DIR}":/fixtures garethr/kubeval "/fixtures/${TEST_NAME}.yaml" --kubernetes-version="$KUBE_VERSION" --ignore-missing-schemas --quiet
    echo ""
}


# default output
$MG > "${TEMP_DIR}/default.yaml"
check default

# global profile
$MG --profile=global > "${TEMP_DIR}/global.yaml"
check global

# restricted profile
$MG --profile=restricted > "${TEMP_DIR}/restricted.yaml"
check restricted 

# soft-multi-tenancy profile
$MG --profile=soft-multi-tenancy --set=kubeAPIServerIP=1.2.3.4 > "${TEMP_DIR}/soft-multi-tenancy.yaml"
check soft-multi-tenancy

# istio profile
$MG --profile=istio > "${TEMP_DIR}/istio.yaml"
check istio

# testdata file setting all possible values
$MG --values="${SCRIPT_DIR}/testdata/values.yaml" > "${TEMP_DIR}/testdata.yaml"
check testdata






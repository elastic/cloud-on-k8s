#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

# Script to generate an ECK installation manifest

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHART_DIR="${SCRIPT_DIR}/../../deploy/eck-operator"
CRD_CHART_DIR="${CHART_DIR}/charts/eck-operator-crds"

update_chart() {
    local ALL_CRDS="${SCRIPT_DIR}/../../config/crds/all-crds.yaml"
    local VERSION
    VERSION=$(cat "${SCRIPT_DIR}/../../VERSION")

    local SED="sed_gnu"
    if [[ "$OSTYPE" =~ "darwin" ]]; then
        SED="sed_bsd"
    fi

    # Patch the CRDs to add Helm labels
    cp -f "$ALL_CRDS" "${SCRIPT_DIR}/crd_patches/all-crds.yaml"
    kubectl kustomize "${SCRIPT_DIR}/crd_patches" > "${CRD_CHART_DIR}/templates/all-crds.yaml"

    # Update the versions in the main chart
    "$SED" -E "s#appVersion: [0-9]+\.[0-9]+\.[0-9]+.*#appVersion: $VERSION#" "${CHART_DIR}/Chart.yaml"
    "$SED" -E "s#version: [0-9]+\.[0-9]+\.[0-9]+.*#version: $VERSION#" "${CHART_DIR}/Chart.yaml"

    # Update the versions in the CRD chart
    "$SED" -E "s#appVersion: [0-9]+\.[0-9]+\.[0-9]+.*#appVersion: $VERSION#" "${CRD_CHART_DIR}/Chart.yaml"
    "$SED" -E "s#version: [0-9]+\.[0-9]+\.[0-9]+.*#version: $VERSION#" "${CRD_CHART_DIR}/Chart.yaml"
}

sed_gnu() {
    sed -i "$@"
}

sed_bsd() {
    sed -i '' "$@"
}

usage() {
    echo "Usage: $0 [-u | -g <args>]"
    echo "    '-u'"
    echo "         Update the chart (version and CRDs) and exit"
    echo "    '-g'"
    echo "         Generate manifest using the given arguments"
    echo ""
    echo "Example: $0 -g --profile=restricted --set=operator.namespace=myns"
    exit 2
}


while getopts "ug" OPT; do
    case "$OPT" in
        u)
            update_chart
            exit 0
            ;;
        g)
            update_chart
            shift $((OPTIND-1))
            ARGS=("$@")
            (
                cd "$SCRIPT_DIR"
                go run main.go --source="$CHART_DIR" generate "${ARGS[@]}"
            )
            exit 0
            ;;
        *)
            usage
            ;;
    esac
done

usage

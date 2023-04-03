#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to set environment variables for building operator and e2e-tests image depending on the context.

set -euo pipefail

WD="$(cd "$(dirname "$0")"; pwd)"
ROOT="$WD/../../.."
# shellcheck disable=SC1091
source "$WD/../common/lib.sh"

help() {
    echo '
    Usage: setenv.sh
    
    Set environment variables to dynamically build ECK based on the following environment variables:
        BUILDKITE_BUILD_NUMBER
        BUILDKITE_TAG
        BUILDKITE_BRANCH
        BUILDKITE_SOURCE
        BUILDKITE_PULL_REQUEST
        GITHUB_PR_TRIGGER_COMMENT

    Optional environment variable:
        BUILD_LICENSE_PUBKEY    to build the operator with a give license public key
'
}

init_env() { :> "$ROOT/.env"; }
set_env()  { echo "$@" | tee -a "$ROOT/.env"; }

main() {
    init_env

    if is_not_buildkite; then
        echo "# dev mode"
        echo export BUILDKITE_PULL_REQUEST=dev
        export BUILDKITE_PULL_REQUEST=dev
    fi

    sha1=$(git rev-parse --short=8 --verify HEAD)
    version=$(cat "$ROOT/VERSION")

    # common vars
    
    # VAULT_* vars are used by hack/docker.sh
    set_env export VAULT_CLIENT_TIMEOUT=120
    
    set_env E2E_REGISTRY_NAMESPACE=eck-ci
    
    set_env GO_TAGS=release
    set_env export LICENSE_PUBKEY=in-memory

    # operator image name vars depending on the trigger
    
    if is_tag; then
        REGISTRY_NAMESPACE=eck
        IMG_SUFFIX=""
        # remove v prefix from the tag
        IMG_VERSION="${BUILDKITE_TAG#v}"

        set_env SNAPSHOT=false
        set_env PUBLISH_IMAGE_UBI=true

    elif is_merge_main || is_nightly_main; then
        REGISTRY_NAMESPACE=eck-snapshots
        IMG_SUFFIX=""
        IMG_VERSION="$version-$sha1"
   
    elif is_pr; then
        REGISTRY_NAMESPACE=eck-ci
        IMG_SUFFIX="-pr"
        IMG_VERSION="$BUILDKITE_PULL_REQUEST-$sha1"

        set_env BUILD_PLATFORM=linux/amd64

    else # any branch
        REGISTRY_NAMESPACE=eck-ci
        IMG_SUFFIX="-branch"
        IMG_VERSION="$version-$sha1"
    fi

    set_env "REGISTRY_NAMESPACE=$REGISTRY_NAMESPACE"
    set_env "IMG_SUFFIX=$IMG_SUFFIX"
    set_env "IMG_VERSION=$IMG_VERSION${OPERATOR_VERSION_SUFFIX:+-$OPERATOR_VERSION_SUFFIX}"
}

main "$@"

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

set_env()  { echo "$@" ; }

# defaut variables
GO_TAGS=release
CONTAINERFILE_PATH=Dockerfile
ECK_VERSION=$(cat "$ROOT/VERSION")
SHA1=$(git rev-parse --short=8 --verify HEAD)
SNAPSHOT=true

main() {

    if is_not_buildkite; then
        echo "# 🧪 dev mode (export BUILDKITE_PULL_REQUEST=dev)"
        export BUILDKITE_PULL_REQUEST=dev
    fi

    # common vars
    
    # VAULT_* vars are used by hack/docker.sh
    set_env export VAULT_CLIENT_TIMEOUT=120
    set_env E2E_REGISTRY_NAMESPACE=eck-ci

    # operator image name vars depending on the trigger

    if is_tag; then
        REGISTRY_NAMESPACE=eck
        IMG_SUFFIX=""
        # remove v prefix from the tag
        IMG_VERSION="${BUILDKITE_TAG#v}"

        SNAPSHOT=false
        #set_env PUBLISH_IMAGE_UBI=true

    elif is_merge_main || is_nightly_main; then
        REGISTRY_NAMESPACE=eck-snapshots
        IMG_SUFFIX=""
        IMG_VERSION="$ECK_VERSION-$SHA1"
   
    elif is_pr; then
        REGISTRY_NAMESPACE=eck-ci
        IMG_SUFFIX="-pr"
        IMG_VERSION="$BUILDKITE_PULL_REQUEST-$SHA1"

    else # any branch
        REGISTRY_NAMESPACE=eck-ci
        IMG_SUFFIX="-branch"
        IMG_VERSION="$ECK_VERSION-$SHA1"
    fi


    buildType=$(buildkite-agent meta-data get BUILD_TYPE)

    case "$buildType" in
        "eck") ;;
        "eck-dev") ;;
        "eck-ubi") ;;
        "eck-fips") ;;
        "eck-ubi-fips") ;;
        "eck-dev") ;;
    ;;


    IMG_NAME=eck-operator
    if [[ "${ENABLE_UBI_BUILD:-false}" == true ]]; then
        IMG_NAME="$IMG_NAME-ubi8"
        CONTAINERFILE_PATH=Dockerfile-ubi
    fi
    if [[ "${ENABLE_FIPS_BUILD:-false}" == true ]]; then
        IMG_NAME="$IMG_NAME-fips"
        GO_TAGS="$GO_TAGS,goexperiment.boringcrypto"
    fi

    if [[ "${OPERATOR_VERSION_SUFFIX:-}" != "" ]]; then
        IMG_VERSION="$IMG_VERSION-$OPERATOR_VERSION_SUFFIX"
    fi

    set_env "CONTAINERFILE_PATH=$CONTAINERFILE_PATH"
    set_env "DOCKER_IMAGE=docker.elastic.co/$REGISTRY_NAMESPACE/$IMG_NAME$IMG_SUFFIX"
    set_env "DOCKER_IMAGE_TAG=$IMG_VERSION"
    set_env "GO_TAGS=$GO_TAGS"
    set_env "ECK_VERSION=$ECK_VERSION"
    set_env "SHA1=$SHA1"
    set_env "SNAPSHOT=$SNAPSHOT"    
}

main "$@"

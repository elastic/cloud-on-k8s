#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Functions to set environment variables to build the operator.

source "$ROOT/.buildkite/scripts/common/lib.sh"

# Sets operator image variables in the environment depending on the given trigger.
operator::set_image_vars() {
    trigger="$1"

    version=$(common::version)
    sha1=$(common::sha1)

    case "$trigger" in
        tag-*)
            : "$BUILDKITE_TAG" # required
            IMAGE_NAME="docker.elastic.co/eck/eck-operator"
            IMAGE_TAG="${BUILDKITE_TAG#v}" # remove v prefix
        ;;
        *-main)
            IMAGE_NAME="docker.elastic.co/eck-snapshots/eck-operator"
            IMAGE_TAG="$version-$sha1"
        ;;
        pr-*)
            : "$BUILDKITE_PULL_REQUEST" # required
            IMAGE_NAME="docker.elastic.co/eck-ci/eck-operator-pr"
            IMAGE_TAG="$BUILDKITE_PULL_REQUEST-$sha1"
        ;;
        dev)
            IMAGE_NAME="docker.elastic.co/eck-dev/eck-operator"
            IMAGE_TAG="dev-$sha1"
        ;;
        *)
            IMAGE_NAME="docker.elastic.co/eck-ci/eck-operator-br"
            IMAGE_TAG="$version-$sha1"
        ;;
    esac

    export IMAGE_NAME
    export IMAGE_TAG
}

# Sets operator BUILD_FLAVORS in the environment depending on the given trigger if it is not set.
operator::set_build_flavors_var() {
    trigger=$1
    if [[ "${BUILD_FLAVORS:-}" == "" ]]; then
        case $trigger in
            tag-*)           BUILD_FLAVORS="eck,eck-dev,eck-fips,eck-ubi,eck-ubi-fips" ;;
            merge-main)      BUILD_FLAVORS="eck,eck-dev" ;;
            nightly-main)    BUILD_FLAVORS="eck,eck-dev,eck-ubi" ;;
            *-test-snapshot) BUILD_FLAVORS="eck,eck-dev" ;;
            pr-*|merge-xyz)  BUILD_FLAVORS="eck" ;;
            dev)             BUILD_FLAVORS="dev" ;;
            *)               echo "error: trigger '$trigger' not supported"; exit ;;
        esac
    fi
    export BUILD_FLAVORS
}

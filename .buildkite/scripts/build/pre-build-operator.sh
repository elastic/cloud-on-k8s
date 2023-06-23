#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to prepare build for eck images by generate drivah config files 
# while detecting the trigger event type and the build flavors.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")"; pwd)/../../.."

main() {

    # hack - waiting for https://github.com/elastic/drivah/pull/73
    rm -f test/e2e/Dockerfile test/e2e/drivah.toml.sh

    auto_detect=$("$ROOT/.buildkite/scripts/common/trigger.sh")

    # get trigger from meta-data first or auto detect it
    TRIGGER=$(buildkite-agent meta-data get TRIGGER --default "${TRIGGER:-$auto_detect}")
    export TRIGGER

    # get build flavors from meta-data or auto detect them from the trigger below
    BUILD_FLAVORS=$(buildkite-agent meta-data get BUILD_FLAVORS --default "${BUILD_FLAVORS:-}")
    export BUILD_FLAVORS

    # set DOCKER_IMAGE/DOCKER_IMAGE_TAG depending on the trigger

    echo -n "# -- env: TRIGGER=$TRIGGER"

    VERSION=$(cat "$ROOT/VERSION")
    SHA1=$(git rev-parse --short=8 --verify HEAD)

    case "$TRIGGER" in
        tag-*)
            : "$BUILDKITE_TAG" # required
            DOCKER_IMAGE="docker.elastic.co/eck/eck-operator"
            DOCKER_IMAGE_TAG="${BUILDKITE_TAG#v}" # remove v prefix
        ;;
        *-main)
            DOCKER_IMAGE="docker.elastic.co/eck-snapshots/eck-operator"
            DOCKER_IMAGE_TAG="$VERSION-$SHA1"
        ;;
        pr-*)
            : "$BUILDKITE_PULL_REQUEST" # required
            DOCKER_IMAGE="docker.elastic.co/eck-ci/eck-operator-pr"
            DOCKER_IMAGE_TAG="$BUILDKITE_PULL_REQUEST-$SHA1"
        ;;
        dev)
            DOCKER_IMAGE="docker.elastic.co/eck-dev/eck-operator"
            DOCKER_IMAGE_TAG="dev-$SHA1"
        ;;
        *)
            DOCKER_IMAGE="docker.elastic.co/eck-ci/eck-operator-br"
            DOCKER_IMAGE_TAG="$VERSION-$SHA1"
        ;;
    esac

    export DOCKER_IMAGE
    export DOCKER_IMAGE_TAG

    # auto-detect flavors depending on the trigger if not set

    if [[ ${BUILD_FLAVORS:-} == "" ]]; then
        case $TRIGGER in
            tag-*)           BUILD_FLAVORS="eck,eck-dev,eck-fips,eck-ubi8,eck-ubi8-fips" ;;
            *-main)          BUILD_FLAVORS="eck,eck-dev" ;;
            *-test-snapshot) BUILD_FLAVORS="eck,eck-dev" ;;
            pr-*|merge-xyz)  BUILD_FLAVORS="eck" ;;
            dev)             BUILD_FLAVORS="dev" ;;
            *)               echo "error: trigger '$TRIGGER' not supported"; exit ;;
        esac
    fi

    export BUILD_FLAVORS

    "$ROOT/build/gen-drivah.toml.sh"
}

main

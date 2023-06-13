#!/bin/bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

#
# Generate drivah.toml to build ECK operator image(s).
#
# Environment variables:
#    BUILD_TRIGGER    tag|merge-main|nightly-main|pr|dev (default: dev)
#    BUILD_FLAVORS    comma separated list (values: dev,eck,eck-dev,eck-fips,eck-ubi8,eck-ubi8-fips) (default: dev)
#

set -euo pipefail

HERE="$(cd "$(dirname "$0")"; pwd)"
ROOT="$HERE/.."

get_meta() {
    local key=$1
    local default_val=$2

    if [[ "${CI:-}" == "true" ]]; then
        buildkite-agent meta-data get "$key" &> v
        status=$? val=$(cat v); rm v
        [[ "$status" == 0 ]] && echo "$val" || echo "$default_val"
    else
        echo "$default_val"
    fi
}

generate_drivah_config() {
    local img=$1
    local tag=$2
cat <<END
[docker]

[buildah]
build_flags = [ "../../"]

[container.image]
names = ["${img}"]
tags = ["${tag}-${ARCH}"]
build_context = "../../"

[container.image.build_args]
VERSION = "${ECK_VERSION}"
SHA1 = "${SHA1}"
GO_TAGS = "${GO_TAGS}"
SNAPSHOT = "${SNAPSHOT}"
LICENSE_PUBKEY_PATH = "build/$LICENSE_PUBKEY"
END
}

ARCH=$(uname -m | sed -e "s|x86_|amd|" -e "s|aarch|arm|")
ECK_VERSION=$(cat "$ROOT/VERSION")
SHA1=$(git rev-parse --short=8 --verify HEAD)
SNAPSHOT=true

main() {
    # set DOCKER_IMAGE/DOCKER_IMAGE_TAG depending on the trigger

    BUILD_TRIGGER="${BUILD_TRIGGER:-dev}"
    BUILD_TRIGGER=$(get_meta BUILD_TRIGGER "${BUILD_TRIGGER}")
    
    echo -n "# -- env: BUILD_TRIGGER=$BUILD_TRIGGER"

    case "$BUILD_TRIGGER" in
        tag-*)
            : "$BUILDKITE_TAG" # required
            DOCKER_IMAGE="docker.elastic.co/eck/eck-operator"
            DOCKER_IMAGE_TAG="${BUILDKITE_TAG#v}" # remove v prefix
            SNAPSHOT=false
        ;;
        *-main)
            DOCKER_IMAGE="docker.elastic.co/eck-snapshots/eck-operator"
            DOCKER_IMAGE_TAG="$ECK_VERSION-$SHA1"
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
            DOCKER_IMAGE_TAG="$ECK_VERSION-$SHA1"
        ;;
    esac

    BUILD_FLAVORS=$(get_meta BUILD_FLAVORS "${BUILD_FLAVORS:-}")

    # auto-detect flavors depending on the trigger if not set
    
    if [[ ${BUILD_FLAVORS} == "" ]]; then
        case $BUILD_TRIGGER in
            tag-*)           BUILD_FLAVORS="eck,eck-dev,eck-fips,eck-ubi8,eck-ubi8-fips" ;;
            *-main)          BUILD_FLAVORS="eck,eck-dev" ;;
            *-test-snapshot) BUILD_FLAVORS="eck,eck-dev" ;;
            pr-*)            BUILD_FLAVORS="eck" ;;
            dev)             BUILD_FLAVORS="dev" ;;
            *)               echo "error: trigger '$BUILD_TRIGGER' not supported"; exit ;;
        esac
    fi

    # create Dockerfile(s) and drivah.toml(s) depending on the build flavors

    echo " BUILD_FLAVORS=$BUILD_FLAVORS"

    rm -rf build/[eckdv]*

    IFS=","; for flavor in $BUILD_FLAVORS; do

        # default vars reset at each iteration
        CONTAINERFILE_PATH=$HERE/Dockerfile
        GO_TAGS=release
        LICENSE_PUBKEY=license.key
        local img="$DOCKER_IMAGE"
        local tag="$DOCKER_IMAGE_TAG"

        if [[ "$flavor" == "dev" ]]; then
            # build without public license key
            GO_TAGS=
            touch "$HERE/$LICENSE_PUBKEY"
        fi
        if [[ "$flavor" =~ -dev ]]; then
                tag="$tag-dev"
                LICENSE_PUBKEY=dev-license.key
                BUILD_LICENSE_PUBKEY=dev
        fi
        if [[ "$flavor" =~ -ubi8 ]]; then
                img="$img-ubi8"
                CONTAINERFILE_PATH=$HERE/Dockerfile-ubi
        fi
        if [[ "$flavor" =~ -fips ]]; then
                img="$img-fips"
                GO_TAGS="$GO_TAGS,goexperiment.boringcrypto"
        fi

        # store the "eck" flavor image to run e2e tests later with it
        if  [[ "${CI:-}" == "true" ]] && [[ "$flavor" == "eck" ]]; then
            buildkite-agent meta-data set operator-image "$img:$tag"
        fi

        # fetch public license key
        if [[ ! -f "$HERE/$LICENSE_PUBKEY" ]]; then
            vault read -field=${BUILD_LICENSE_PUBKEY:+$BUILD_LICENSE_PUBKEY-}pubkey secret/ci/elastic-cloud-on-k8s/license \
                | base64 --decode > "$HERE/$LICENSE_PUBKEY"
        fi

        # generate drivah.toml and copy Dockerfile
        echo "# -- build: $img:$tag"
        mkdir -p "$HERE/$flavor"
        generate_drivah_config "$img" "$tag" > "$HERE/$flavor/drivah.toml"
        cp -f "$CONTAINERFILE_PATH" "$HERE/$flavor/Dockerfile"

    done
}

main

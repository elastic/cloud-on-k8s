#!/bin/bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Prepare Dockerfile(s) and drivah.toml(s) depending on a list of build flavors to build ECK operator image(s).
#
# Environment variables:
#    DOCKER_IMAGE       base Docker image
#    DOCKER_IMAGE_TAG   base Docker image tag
#    BUILD_FLAVORS      comma separated list (values: dev,eck,eck-dev,eck-fips,eck-ubi8,eck-ubi8-fips) (default: dev)
#

set -euo pipefail

HERE="$(cd "$(dirname "$0")"; pwd)"
ROOT="$HERE/.."

retry() { "$ROOT/hack/retry.sh" 5 "$@"; }

: "$DOCKER_IMAGE"
: "$DOCKER_IMAGE_TAG"
: "$BUILD_FLAVORS"

ARCH=$(uname -m | sed -e "s|x86_|amd|" -e "s|aarch|arm|")
VERSION=$(cat "$ROOT/VERSION")
SHA1=$(git rev-parse --short=8 --verify HEAD)
GO_TAGS=release
LICENSE_PUBKEY=license.key
SNAPSHOT=true

generate_drivah_config() {
    local image=$1
    local image_tag=$2
    local go_tags=$3
    local license_pub_key=$4
cat <<END
[container.image]
names = ["${image}"]
tags = ["${image_tag}-${ARCH}"]
build_context = "../../"

[container.image.build_args]
VERSION = "${VERSION}"
SHA1 = "${SHA1}"
GO_TAGS = "${go_tags}"
SNAPSHOT = "${SNAPSHOT}"
LICENSE_PUBKEY_PATH = "build/$license_pub_key"
END
}

main() {
    echo "#-- gen-drivah-config BUILD_FLAVORS=$BUILD_FLAVORS"

    # disable SNAPSHOT for tags
    tag_pattern="^[0-9]+\.[0-9]+\.[0-9]+"
    if [[ "$DOCKER_IMAGE_TAG"  =~ $tag_pattern ]]; then
        SNAPSHOT=false
    fi

    rm -rf build/[eckdv]*

    IFS=","; for flavor in $BUILD_FLAVORS; do

        # default vars reset at each iteration
        container_file_path=$HERE/Dockerfile
        go_tags=$GO_TAGS
        license_pubkey=$LICENSE_PUBKEY

        image="$DOCKER_IMAGE"
        image_tag="$DOCKER_IMAGE_TAG"

         # dev build without public license key
        if [[ "$flavor" == "dev" ]]; then
            go_tags=
            echo "fake empty license" > "$HERE/$LICENSE_PUBKEY"
        fi
        # DEV license public key build
        if [[ "$flavor" =~ -dev ]]; then
                image_tag="$image_tag-dev"
                license_pubkey=dev-license.key
                BUILD_LICENSE_PUBKEY=dev
        fi
        # UBI8 build
        if [[ "$flavor" =~ -ubi8 ]]; then
                image="$image-ubi8"
                container_file_path=$HERE/Dockerfile-ubi
        fi
        # FIPS build
        if [[ "$flavor" =~ -fips ]]; then
                image="$image-fips"
                go_tags="$go_tags,goexperiment.boringcrypto"
        fi

        # store the "eck" operator image to run e2e tests later with it
        if  [[ "${CI:-}" == "true" ]] && [[ "$flavor" == "eck" ]]; then
            buildkite-agent meta-data set operator-image "$image:$image_tag"
        fi

        # fetch public license key
        if [[ ! -f "$HERE/$license_pubkey" ]]; then
            prefix="${BUILD_LICENSE_PUBKEY:+$BUILD_LICENSE_PUBKEY-}" # add "-" suffix
            retry vault read -field="${prefix}pubkey" secret/ci/elastic-cloud-on-k8s/license \
                | base64 --decode > "$HERE/$license_pubkey"
        fi

        # generate drivah.toml and copy Dockerfile
        echo "# -- build: $image:$image_tag"
        mkdir -p "$HERE/$flavor"
        generate_drivah_config "$image" "$image_tag" "$go_tags" "$license_pubkey" > "$HERE/$flavor/drivah.toml"
        cp -f "$container_file_path" "$HERE/$flavor/Dockerfile"
    done
}

main

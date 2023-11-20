#!/bin/bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Prepare Dockerfile(s) and drivah.toml(s) to build ECK operator container image(s).

set -euo pipefail

HERE="$(cd "$(dirname "$0")"; pwd)"
ROOT="$HERE/.."

source "$ROOT/.buildkite/scripts/common/lib.sh"

retry() { "$ROOT/hack/retry.sh" 5 "$@"; }

: "$IMAGE_NAME"
: "$IMAGE_TAG"
: "$BUILD_FLAVORS"

ARCH=$(common::arch)
SHA1=$(common::sha1)
VERSION=$(common::version)

SNAPSHOT=true
GO_TAGS=${GO_TAGS-release}
LICENSE_PUBKEY=license.key

generate_drivah_config() {
    local name=$1
    local tag=$2
    local go_tags=$3
    local license_pubkey=$4
cat <<END
[container.image]
names = ["${name}"]
tags = ["${tag}-${ARCH}"]
build_context = "../../"

[container.image.build_args]
VERSION = "${VERSION}"
SHA1 = "${SHA1}"
GO_TAGS = "${go_tags}"
SNAPSHOT = "${SNAPSHOT}"
LICENSE_PUBKEY = "$license_pubkey"
END
}

main() {
    echo "# -- gen-drivah-config BUILD_FLAVORS=$BUILD_FLAVORS"

    # disable SNAPSHOT for tags
    tag_pattern="^[0-9]+\.[0-9]+\.[0-9]+"
    if [[ "$IMAGE_TAG"  =~ $tag_pattern ]]; then
        SNAPSHOT=false
    fi

    # delete only dirs
    find "$HERE" -maxdepth 1 -mindepth 1 -type d -exec rm -rf '{}' \;

    IFS=","; for flavor in $BUILD_FLAVORS; do

        # default vars reset at each iteration
        container_file_path=$HERE/Dockerfile
        go_tags=$GO_TAGS
        license_pubkey=$LICENSE_PUBKEY

        name="$IMAGE_NAME"
        tag="$IMAGE_TAG"

         # dev build without public license key
        if [[ "$flavor" == "dev" ]]; then
            go_tags=
            echo "fake empty license" > "$HERE/$LICENSE_PUBKEY"
        fi
        # DEV license public key build
        if [[ "$flavor" =~ -dev ]]; then
                tag="$tag-dev"
                license_pubkey=dev-license.key
                BUILD_LICENSE_PUBKEY=dev
        fi
        # UBI build
        if [[ "$flavor" =~ -ubi ]]; then
                name="$name-ubi"
                container_file_path=$HERE/Dockerfile-ubi
        fi
        # FIPS build
        if [[ "$flavor" =~ -fips ]]; then
                name="$name-fips"
                go_tags="$go_tags,goexperiment.boringcrypto"
        fi

        # fetch public license key
        if [[ ! -f "$HERE/$license_pubkey" ]]; then
            prefix="${BUILD_LICENSE_PUBKEY:+$BUILD_LICENSE_PUBKEY-}" # add "-" suffix
            retry vault read -field="${prefix}pubkey" secret/ci/elastic-cloud-on-k8s/license \
                | base64 --decode > "$HERE/$license_pubkey"
        fi

        # generate drivah.toml and copy Dockerfile
        echo "# -- build: $name:$tag"
        mkdir -p "$HERE/$flavor"
        generate_drivah_config "$name" "$tag" "$go_tags" "$license_pubkey" > "$HERE/$flavor/drivah.toml"
        cp -f "$container_file_path" "$HERE/$flavor/Dockerfile"

    done
}

main

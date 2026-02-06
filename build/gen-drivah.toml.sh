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

latest_stable_tag() {
    case "${TRIGGER:-}" in
        merge-xyz) echo next-release-latest ;;
        tag-bc)    echo bc-latest ;;
        *)         echo latest ;;
    esac
}

generate_drivah_config() {
    local name=$1
    local tag=$2
    local go_tags=$3
    local license_pubkey=$4

    # add 'stable' tag without sha1 for snapshots
    if [[ "$tag" =~ "SNAPSHOT" ]]; then
        snapshot_stable_tag="${tag/-$SHA1/}"
        additional_tags=",\"${snapshot_stable_tag}-${ARCH}\",\"$(latest_stable_tag)-${ARCH}\""
    else
        additional_tags=",\"$(latest_stable_tag)-${ARCH}\""
    fi

cat <<END
[container.image]
names = ["${name}"]
tags = ["${tag}-${ARCH}"${additional_tags:-}]
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
    echo "# -- gen-drivah-config BUILD_FLAVORS=$BUILD_FLAVORS TRIGGER=$TRIGGER"

    # disable SNAPSHOT for tags
    tag_pattern="^[0-9]+\.[0-9]+\.[0-9]+"
    if [[ "$IMAGE_TAG"  =~ $tag_pattern ]]; then
        SNAPSHOT=false
    fi

    # delete only dirs
    find "$HERE" -maxdepth 1 -mindepth 1 -type d -exec rm -rf '{}' \;

    # initialize file to share list of images for CVE scan and signing
    true > images-to-scan.txt
    true > images-to-sign.txt

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

        # write the image name with the latest stable tag (except the 'dev' flavor) for CVE scan
        if [[ ! "$flavor" =~ -dev ]]; then
            echo "$name:$(latest_stable_tag)" >> images-to-scan.txt
        fi

        # write the image name for signing (using the multi-arch manifest)
        echo "$name:${tag}" >> images-to-sign.txt

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

    if [[ "${CI:-}" == true ]]; then
        buildkite-agent meta-data set images-to-scan "$(cat images-to-scan.txt)"
        buildkite-agent meta-data set images-to-sign "$(cat images-to-sign.txt)"
    fi
}

main

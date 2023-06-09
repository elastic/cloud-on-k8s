#!/bin/bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

#
# Build ECK operator image(s).
#
# Environment variables:

#    BUILD_TRIGGER    tag|merge-main|nightly-main|pr (default: pr)
#    BUILD_FLAVORS    comma separated list (values: eck,eck-fips,eck-ubi8,eck-ubi8-fips,eck-dev) (default: dev)
#

set -uo pipefail
ROOT="$(cd "$(dirname "$0")"; pwd)"

generate_drivah_config() {
    local img=$1
    local tag=$2
    local build_context=$3
cat <<EOF
#!/bin/bash

get_arch() { uname -m | sed -e "s|x86_|amd|" -e "s|aarch|arm|"; }

get_go_ldflags() {
    echo "-X github.com/elastic/cloud-on-k8s/v2/pkg/about.version=${ECK_VERSION} \
        -X github.com/elastic/cloud-on-k8s/v2/pkg/about.buildHash=${SHA1} \
        -X github.com/elastic/cloud-on-k8s/v2/pkg/about.buildDate=$(date -u +'%Y-%m-%dT%H:%M:%SZ') \
        -X github.com/elastic/cloud-on-k8s/v2/pkg/about.buildSnapshot=${SNAPSHOT}"
}

cat <<END
[docker]

[buildah]
build_flags = ["${build_context}"]

[container.image]
names = ["${img}"]
tags = ["${tag}-\$(get_arch)"]

[container.image.build_args]
GO_LDFLAGS = "\$(get_go_ldflags)"
GO_TAGS = "${GO_TAGS}"
LICENSE_PUBKEY_PATH = "${LICENSE_PUBKEY_PATH}"
END

if [[ ! -d build ]]; then
echo "[container.image.hooks]"
echo "pre_build = \"buildkite-agent artifact download 'build/**' .\""
fi

EOF
}

get_meta() {
    buildkite-agent meta-data get $1 2> /dev/null > v
    local meta_status=$?
    local meta_val=$(cat v); rm v
    local default_val=$2
    if [[ "$meta_status" == 0 ]]; then
        echo "$meta_val"
    else
        echo "$default_val"
    fi
}

# default values
BUILD_TRIGGER="${BUILD_TRIGGER:-dev}"
BUILD_FLAVORS="${BUILD_FLAVORS:-dev}"

## set DOCKER_IMAGE/DOCKER_IMAGE_TAG depending on the trigger

ECK_VERSION=$(cat "$ROOT/VERSION")
SHA1=$(git rev-parse --short=8 --verify HEAD)
SNAPSHOT=true

BUILD_TRIGGER=$(get_meta BUILD_TRIGGER "${BUILD_TRIGGER}")
echo -n "# -- BUILD_TRIGGER=$BUILD_TRIGGER"

case "$BUILD_TRIGGER" in       
    tag) # depends on BUILDKITE_TAG
        DOCKER_IMAGE="docker.elastic.co/eck/eck-operator"
        DOCKER_IMAGE_TAG="${BUILDKITE_TAG#v}"
        SNAPSHOT=false
    ;;
    merge-main|nightly-main)
        DOCKER_IMAGE="docker.elastic.co/eck-snapshots/eck-operator"
        DOCKER_IMAGE_TAG="$ECK_VERSION-$SHA1"
    ;;
    dev)
        DOCKER_IMAGE="docker.elastic.co/eck-ci/eck-operator-dev"
        DOCKER_IMAGE_TAG="dev-$SHA1"
    ;;
    pr) # depends on BUILDKITE_PULL_REQUEST
        DOCKER_IMAGE="docker.elastic.co/eck-ci/eck-operator-pr"
        DOCKER_IMAGE_TAG="$BUILDKITE_PULL_REQUEST-$SHA1"
    ;;
    *)
        DOCKER_IMAGE="docker.elastic.co/eck-ci/eck-operator-br"
        DOCKER_IMAGE_TAG="$ECK_VERSION-$SHA1"
    ;;
esac

## create Dockerfile(s) and drivah.toml(s) depending on the build flavors

#rm -rf build/

BUILD_FLAVORS=$(get_meta BUILD_FLAVORS "${BUILD_FLAVORS}")
echo " BUILD_FLAVORS=$BUILD_FLAVORS"

IFS=","; for t in $BUILD_FLAVORS; do
    
    # default vars
    LICENSE_PUBKEY=license.key
    GO_TAGS=release
    CONTAINERFILE_PATH=$ROOT/Dockerfile
    
    img="$DOCKER_IMAGE"
    tag="$DOCKER_IMAGE_TAG"

    if [[ "$t" =~ -dev ]]; then
            tag="$tag-dev"
            LICENSE_PUBKEY=dev-license.key
            BUILD_LICENSE_PUBKEY=dev
    fi
    if [[ "$t" =~ -ubi8 ]]; then
            img="$img-ubi8"
            CONTAINERFILE_PATH=$ROOT/Dockerfile-ubi
    fi
    if [[ "$t" =~ -fips ]]; then
            img="$img-fips"
            GO_TAGS="$GO_TAGS,goexperiment.boringcrypto"
    fi

    # fetch public license key
    LICENSE_PUBKEY_PATH=$LICENSE_PUBKEY
    if [[ "$LICENSE_PUBKEY" != "" ]] && [[ ! -f "$LICENSE_PUBKEY" ]]; then
        vault read -field=${BUILD_LICENSE_PUBKEY:+$BUILD_LICENSE_PUBKEY-}pubkey secret/ci/elastic-cloud-on-k8s/license | base64 --decode > $LICENSE_PUBKEY_PATH
    fi

    # dev mode
    if [[ "$t" == "dev" ]]; then
            LICENSE_PUBKEY= GO_TAGS=
            generate_drivah_config "$DOCKER_IMAGE-dev" "$DOCKER_IMAGE_TAG" "." | bash
            exit 0
    fi

    if [[ "$t" == "eck" ]]; then
        generate_drivah_config "$img" "$tag" "." | bash
    else
        # copy Dockerfile and generate drivah.toml
        echo "# -- build $img:$tag"
        mkdir -p build/$t
        generate_drivah_config "$img" "$tag" "../../" > build/$t/drivah.toml.sh
        chmod +x build/$t/drivah.toml.sh
        cp -f $CONTAINERFILE_PATH build/$t/Dockerfile
    fi
done

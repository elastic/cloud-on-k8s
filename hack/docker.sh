#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to handle exoticisms related to 'docker login' and 'docker push'.
#
# Log in to docker.elastic.co if the namespace eck, eck-ci or eck-snapshots is used
# Log in to gcloud if GCR is used

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

retry() { "$SCRIPT_DIR/retry.sh" 5 "$@"; }

docker-login() {
    local image=$1
    local registry=${image%%"/"*}

    # check only performs in dev because it doesn't work very well in CI
    if [[ -z "${CI:-}" && -f ~/.docker/config.json ]] && grep -q "${registry}" ~/.docker/config.json; then
        echo "not authenticating to ${registry} as configuration block already exists in ~/.docker/config.json"
        return
    fi

    case "$image" in

        docker.elastic.co/*)
            DOCKER_LOGIN=$(retry vault read -field=username "${VAULT_ROOT_PATH}/docker-registry-elastic")
            DOCKER_PASSWORD=$(retry vault read -field=password "${VAULT_ROOT_PATH}/docker-registry-elastic")

            echo "Authentication to ${registry}..."
            docker login -u "${DOCKER_LOGIN}" -p "${DOCKER_PASSWORD}" docker.elastic.co 2> /dev/null
        ;;

        *.gcr.io/*)
            echo "Authentication to ${registry}..."
            gcloud auth configure-docker --quiet 2> /dev/null
        ;;

        *)
            if ! grep -q "$registry" ~/.docker/config.json; then
               echo "Please log in to $registry."
               exit 1
            fi
        ;;
    esac
}

docker-push() {
    local image=$1
    echo "Push $image..."
    # silence the verbose output of the `docker push` command
    retry \
        docker push "$image" | grep -v -E 'Waiting|Layer already|Preparing|Pushing|Pushed'
}

docker-multiarch-init() {
    local BUILDER_NAME="eck-multi-arch"
    docker buildx create --driver docker-container --name "$BUILDER_NAME" --platform linux/amd64,linux/arm64 --use >/dev/null 2>&1 || echo "$BUILDER_NAME already exists"
    docker run --rm --privileged multiarch/qemu-user-static --reset -p yes >/dev/null 2>&1 
}

usage() {
    echo "Usage: $0 <-l | -m | -p> image"
    echo "  -l   Login to registry"
    echo "  -m   Configure system for multi-arch build"
    echo "  -p   Push to registry"
    exit 2
}


OPT_LOGIN="no"
OPT_PUSH="no"
OPT_MULTI_ARCH="no"

while getopts ":lpm" OPT; do
    case "$OPT" in
        l)
            OPT_LOGIN="yes"
            ;;
        m)
            OPT_MULTI_ARCH="yes"
            ;;
        p)
            OPT_PUSH="yes"
            ;;
        \?) 
            usage
            ;;
        *)
            usage
            ;;
    esac
done

shift $((OPTIND - 1))

if [[ ! $# -eq 1 ]]; then
    usage
fi

echo ">> Image == $1"

if [[ "$OPT_MULTI_ARCH" == "yes" ]]; then
    docker-multiarch-init
fi

if [[ "$OPT_LOGIN" == "yes" ]]; then
    docker-login "$1"
fi

if [[ "$OPT_PUSH" == "yes" ]]; then
    docker-push "$1"
fi


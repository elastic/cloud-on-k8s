#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to publish ECK operator image from docker.elastic.co registry to docker.io registry (aka Docker Hub).

set -eu

install_docker_extension() {
    [[ ! -f ~/.docker/cli-plugins/docker-buildx ]] && return

    DOCKER_BUILDX_VERSION=0.8.2
    mkdir -p ~/.docker/cli-plugins
    curl -fsSLo ~/.docker/cli-plugins/docker-buildx https://github.com/docker/buildx/releases/download/v${DOCKER_BUILDX_VERSION}/buildx-v${DOCKER_BUILDX_VERSION}.linux-arm64
    chmod a+x ~/.docker/cli-plugins/docker-buildx
}

vault_login() {
    VAULT_TOKEN=$(vault write -field=token auth/approle/login role_id="${VAULT_ROLE_ID}" secret_id="${VAULT_SECRET_ID}")
    export VAULT_TOKEN
}

registry_login() {
    DOCKERHUB_LOGIN=$(vault read -field=username secret/release/docker-hub-eck)
    DOCKERHUB_PASSWORD=$(vault read -field=token secret/release/docker-hub-eck)

    docker login -u "${DOCKERHUB_LOGIN}" -p "${DOCKERHUB_PASSWORD}" 2> /dev/null
}

publish() {
    local name=$1
    docker buildx imagetools create -t "$REGISTRY_DST/$name:$ECK_VERSION" "$REGISTRY_SRC/$name:$ECK_VERSION"
}

# main

if [[ "${ECK_VERSION:-}" == "" ]]; then
    echo "ECK_VERSION must be set"
    exit 1
fi

REGISTRY_SRC="docker.elastic.co/eck"
REGISTRY_DST="docker.elastic.co/eck-dev"

if [[ "${DRY_RUN:-}" == "false" ]]; then
    REGISTRY_DST="docker.io/elastic"
fi

install_docker_extension
vault_login
registry_login

publish eck-operator
publish eck-operator-ubi8
publish eck-operator-fips
publish eck-operator-ubi8-fips

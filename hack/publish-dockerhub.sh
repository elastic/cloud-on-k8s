#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to publish ECK operator image from docker.elastic.co/eck registry to docker.io/elastic registry (aka Docker Hub).
# By default, the script is executed with DRY_RUN=true and images are published to docker.elastic.co/eck-ci registry.

set -eu

install_docker_extension() {
    [[ -f ~/.docker/cli-plugins/docker-buildx ]] && return

    DOCKER_BUILDX_VERSION=0.10.4
    mkdir -p ~/.docker/cli-plugins
    curl -fsSLo ~/.docker/cli-plugins/docker-buildx https://github.com/docker/buildx/releases/download/v${DOCKER_BUILDX_VERSION}/buildx-v${DOCKER_BUILDX_VERSION}.linux-amd64
    chmod a+x ~/.docker/cli-plugins/docker-buildx
}

vault_login() {
    VAULT_TOKEN=$(vault write -field=token auth/approle/login role_id="${VAULT_ROLE_ID}" secret_id="${VAULT_SECRET_ID}")
    export VAULT_TOKEN
}

registry_login() {
    if [[ "${DRY_RUN:-}" == "false" ]]; then
        username=$(vault read -field=username secret/release/docker-hub-eck)
        password=$(vault read -field=token    secret/release/docker-hub-eck)
    else
        username=$(vault read -field=username secret/devops-ci/cloud-on-k8s/docker-registry-elastic)
        password=$(vault read -field=password secret/devops-ci/cloud-on-k8s/docker-registry-elastic)
    fi

    docker login -u "$username" -p "$password" "$REGISTRY_DST" 2> /dev/null
}

publish() {
    local image_name=$1

    docker buildx imagetools create \
        -t "$REGISTRY_DST/$NAMESPACE_DST/$image_name:$ECK_VERSION" \
        "$REGISTRY_SRC/$NAMESPACE_SRC/$image_name:$ECK_VERSION"
}

# main

# source of images to copy
REGISTRY_SRC=docker.elastic.co
NAMESPACE_SRC=eck

# destination of images to copy
# default values of dry run
REGISTRY_DST=docker.elastic.co
NAMESPACE_DST=eck-ci
# dockerhub values for live execution
if [[ "${DRY_RUN:-}" == "false" ]]; then
    REGISTRY_DST="docker.io"
    NAMESPACE_DST=elastic
fi

if [[ "${ECK_VERSION:-}" == "" ]]; then
    echo "ECK_VERSION must be set"
    exit 1
fi

install_docker_extension
vault_login
registry_login

publish eck-operator
publish eck-operator-ubi8
publish eck-operator-fips
publish eck-operator-ubi8-fips

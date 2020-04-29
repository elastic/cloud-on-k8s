#! /usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

# Helper to handle docker login and docker push.

set -eu

if [[ -f .registry.env ]]; then
    export $(cat .registry.env | sed "s|\s*=\s*|=|g" | xargs) > /dev/null
fi

docker-login() {
    local image=$1
    local registry=${image%%"/"*}

    if grep -q "$registry" ~/.docker/config.json; then
        # already log in
        return 0
    fi

    echo "Authentication to ${registry}..."
    case ${registry} in

        docker.elastic.co)
            docker login -u "${DOCKER_LOGIN}" -p "${DOCKER_PASSWORD}" push.docker.elastic.co 2> /dev/null
        ;;

        *.gcr.io)
            gcloud auth configure-docker --quiet
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

    # Add the 'push.' prefix for docker.elastic.co
    case ${image} in

        docker.elastic.co/*)
            docker tag "$image" "push.$image"
            image="push.$image"
        ;;

    esac

    echo "Push $image..."
    docker push "$image" | grep -v -E 'Waiting|Layer already|Preparing|Pushing|Pushed'
}

docker-login "$@"
docker-push  "$@"

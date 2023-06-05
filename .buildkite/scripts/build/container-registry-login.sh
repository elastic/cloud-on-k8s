#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to logged in to docker.elastic.co registry.

set -eu

buildah login \
    --username="$(vault read -field=username secret/ci/elastic-cloud-on-k8s/docker-registry-elastic)" \
    --password="$(vault read -field=password secret/ci/elastic-cloud-on-k8s/docker-registry-elastic)" \
    docker.elastic.co
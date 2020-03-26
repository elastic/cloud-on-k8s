#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

# This script is used to pre-pull Docker images into the immutable image of Jenkins workers.

set -eou pipefail

# ECK CI tooling image
docker pull "$(make -C .ci --no-print-directory print-ci-image)"

# Kind images (https://hub.docker.com/r/kindest/node/tags)
docker pull kindest/node:v1.12.10
docker pull kindest/node:v1.16.4
docker pull kindest/node:v1.17.0

# Elastic Stack images
docker pull docker.elastic.co/elasticsearch/elasticsearch:7.6.0
docker pull docker.elastic.co/kibana/kibana:7.6.0
docker pull docker.elastic.co/apm/apm-server:7.6.0

#! /usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

# This script used to "bake" Docker images into base image for Jenkins nodes.

set -eou pipefail

declare -a docker_images=("docker.elastic.co/eck/eck-ci:57e169ff162c2ea351a1956e1ad355b9")

# Pull all the required docker images
for image in "${docker_images[@]}"
do
  docker pull $image
done

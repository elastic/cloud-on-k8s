#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

# This script is used to pre-pull Docker images into the immutable image of Jenkins workers.

set -eou pipefail

# ECK CI tooling image
docker pull "$(make -C .ci --no-print-directory print-ci-image)"

# Kind images (https://hub.docker.com/r/kindest/node/tags)
# Pull exact images due to image incompatibility between kind 0.8 and 0.9 see https://github.com/kubernetes-sigs/kind/releases
docker pull kindest/node:v1.12.10@sha256:faeb82453af2f9373447bb63f50bae02b8020968e0889c7fa308e19b348916cb
docker pull kindest/node:v1.13.12@sha256:214476f1514e47fe3f6f54d0f9e24cfb1e4cda449529791286c7161b7f9c08e7
docker pull kindest/node:v1.14.10@sha256:6033e04bcfca7c5f2a9c4ce77551e1abf385bcd2709932ec2f6a9c8c0aff6d4f
docker pull kindest/node:v1.15.12@sha256:8d575f056493c7778935dd855ded0e95c48cb2fab90825792e8fc9af61536bf9
docker pull kindest/node:v1.16.15@sha256:430c03034cd856c1f1415d3e37faf35a3ea9c5aaa2812117b79e6903d1fc9651
docker pull kindest/node:v1.17.17@sha256:c581fbf67f720f70aaabc74b44c2332cc753df262b6c0bca5d26338492470c17
docker pull kindest/node:v1.18.19@sha256:530378628c7c518503ade70b1df698b5de5585dcdba4f349328d986b8849b1ee
docker pull kindest/node:v1.19.11@sha256:7664f21f9cb6ba2264437de0eb3fe99f201db7a3ac72329547ec4373ba5f5911
docker pull kindest/node:v1.20.7@sha256:e645428988191fc824529fd0bb5c94244c12401cf5f5ea3bd875eb0a787f0fe9
docker pull kindest/node:v1.21.1@sha256:fae9a58f17f18f06aeac9772ca8b5ac680ebbed985e266f711d936e91d113bad


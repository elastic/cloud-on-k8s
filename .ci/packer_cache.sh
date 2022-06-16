#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# This script is used to pre-pull Docker images into the immutable image of Jenkins workers.

set -eou pipefail

# ECK CI tooling image
docker pull "$(make -C .ci --no-print-directory print-ci-image)"

# Kind images (https://hub.docker.com/r/kindest/node/tags)
# Pull exact images due to image incompatibility between kind 0.8 and 0.9 see https://github.com/kubernetes-sigs/kind/releases
docker pull kindest/node:v1.20.15@sha256:6f2d011dffe182bad80b85f6c00e8ca9d86b5b8922cdf433d53575c4c5212248
docker pull kindest/node:v1.21.12@sha256:f316b33dd88f8196379f38feb80545ef3ed44d9197dca1bfd48bcb1583210207
docker pull kindest/node:v1.22.9@sha256:8135260b959dfe320206eb36b3aeda9cffcb262f4b44cda6b33f7bb73f453105
docker pull kindest/node:v1.23.6@sha256:b1fa224cc6c7ff32455e0b1fd9cbfd3d3bc87ecaa8fcb06961ed1afb3db0f9ae
docker pull kindest/node:v1.24.1@sha256:11276ce4763d5600379c0104c4cb51bf7420c8c02937708e958ad68d08e0910c
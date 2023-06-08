#!/bin/bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

set -euo pipefail
ROOT="$(cd "$(dirname "$0")"; pwd)"

# fetch public license key
vault read -field=${BUILD_LICENSE_PUBKEY:+$BUILD_LICENSE_PUBKEY-}pubkey secret/ci/elastic-cloud-on-k8s/license | base64 --decode > license.key

# source env build vars
export $($ROOT/.buildkite/scripts/build/setenv.sh | grep -v "^#" | xargs)

: $CONTAINERFILE_PATH
: $DOCKER_IMAGE
: $DOCKER_IMAGE_TAG
: $GO_TAGS

# for $GO_LDFLAGS
: $ECK_VERSION
: $SHA1
: $SNAPSHOT

get_arch() { uname -m | sed -e "s|x86_|amd|" -e "s|aarch|arm|"; }
get_go_ldflags() {
    echo "-X github.com/elastic/cloud-on-k8s/v2/pkg/about.version=${ECK_VERSION} \
        -X github.com/elastic/cloud-on-k8s/v2/pkg/about.buildHash=${SHA1} \
        -X github.com/elastic/cloud-on-k8s/v2/pkg/about.buildDate=$(date -u +'%Y-%m-%dT%H:%M:%SZ') \
        -X github.com/elastic/cloud-on-k8s/v2/pkg/about.buildSnapshot=${SNAPSHOT}"
}

cat <<EOF
[docker]
build_flags = [
    "--ssh=default",
    "-f=$CONTAINERFILE_PATH"
]

[buildah]
build_flags = [
    "--ssh=default=/etc/ssh/id_rsa",
    "-f=$CONTAINERFILE_PATH"
]

[container.image]
names = ["${DOCKER_IMAGE}"]
tags = ["${DOCKER_IMAGE_TAG}-$(get_arch)"]

[container.image.build_args]
GO_LDFLAGS = "$(get_go_ldflags)"
GO_TAGS = "${GO_TAGS}"
EOF

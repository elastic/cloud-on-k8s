#!/bin/bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")"; pwd)"

get_arch() {
    uname -m | sed -e "s|x86_|amd|" -e "s|aarch|arm|"
}

get_go_ldflags() {
    echo "-X github.com/elastic/cloud-on-k8s/v2/pkg/about.version=$(cat VERSION) \
        -X github.com/elastic/cloud-on-k8s/v2/pkg/about.buildHash=$(git rev-parse --short=8 --verify HEAD) \
        -X github.com/elastic/cloud-on-k8s/v2/pkg/about.buildDate=$(date -u +'%Y-%m-%dT%H:%M:%SZ') \
        -X github.com/elastic/cloud-on-k8s/v2/pkg/about.buildSnapshot=${SNAPSHOT:-true}"
}

# fetch public license key
vault read -field=${SECRET_FIELD_PREFIX:-}pubkey secret/ci/elastic-cloud-on-k8s/license | base64 --decode > license.key

# source env build vars
export $($ROOT/.buildkite/scripts/build/setenv.sh | grep -v "^#" | xargs)

cat <<EOF
[docker]
build_flags = [ "--ssh=default" ]

[buildah]
build_flags = [ "--ssh=default=/etc/ssh/id_rsa" ]

[container.image]
names = ["docker.elastic.co/${REGISTRY_NAMESPACE}/eck-operator"]
tags = ["${IMG_VERSION}-$(get_arch)"]

[container.image.build_args]
GO_LDFLAGS = "$(get_go_ldflags)"
GO_TAGS = "${GO_TAGS}"
VERSION = "$(cat $ROOT/VERSION)"
EOF

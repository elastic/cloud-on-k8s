#!/bin/bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

set -euo pipefail

HERE="$(cd "$(dirname "$0")"; pwd)"
ROOT="$HERE/../.."

retry() { "$ROOT/hack/retry.sh" 5 "$@"; }
arch() { uname -m | sed -e "s|x86_|amd|" -e "s|aarch|arm|"; }
sha1() { git rev-parse --short=8 --verify HEAD; }

main() {
    if [[ ! -f "${HERE}/license.key" ]]; then
        retry vault read -field=pubkey secret/ci/elastic-cloud-on-k8s/license | base64 --decode > "$HERE/license.key"
    fi

    img="docker.elastic.co/eck-ci/eck-e2e-tests"
    tag="$(sha1)"

    # store the image to run e2e tests later with it
    if  [[ "${CI:-}" == "true" ]]; then
        buildkite-agent meta-data set e2e-image "$img:$tag"
    fi

    cat <<END
[container.image]
names = ["$img"]
tags = ["$tag-$(arch)"]
build_context = "../../"

[container.image.build_args]
GO_TAGS = "release"
END

}

main

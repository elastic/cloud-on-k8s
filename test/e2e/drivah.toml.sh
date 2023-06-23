#!/bin/bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to print the drivah.toml config for building the e2e-tests image.

# Usage: ./drival.toml.sh [--env]
#
# Option:
#   --env      print IMAGE_NAME and IMAGE_TAG (optional)


set -euo pipefail

HERE="$(cd "$(dirname "$0")"; pwd)"
ROOT="$HERE/../.."

source "$ROOT/.buildkite/scripts/common/lib.sh"

# can be defined in the env (used by Makefile) 
E2E_IMAGE_NAME=${E2E_IMAGE_NAME:-docker.elastic.co/eck-ci/eck-e2e-tests} # default for CI
E2E_IMAGE_TAG=${E2E_IMAGE_TAG:-$(common::sha1)}
ARCH=$(common::arch)
GO_TAGS=${GO_TAGS-release}

main() {
    # trick to share image to pipeline-gen to start e2e-tests before images are built (via .buildkite/scripts/common/set-images.sh)
    if [[ "${1:-}" == "--env" ]]; then
        echo "E2E_IMAGE_NAME=$E2E_IMAGE_NAME"
        echo "E2E_IMAGE_TAG=$E2E_IMAGE_TAG"
        exit 0
    fi

    if [[ "$GO_TAGS" == "release" ]] && [[ ! -f "${HERE}/license.key" ]]; then
        common::retry vault read -field=pubkey secret/ci/elastic-cloud-on-k8s/license \
            | base64 --decode > "$HERE/license.key"
    else
        touch "$HERE/license.key"
    fi

    # generate drivah config
    cat <<- END
[container.image]
names = ["$E2E_IMAGE_NAME"]
tags = ["$E2E_IMAGE_TAG-$ARCH"]
build_context = "../../"

[container.image.build_args]
GO_TAGS = "$GO_TAGS"
END
}

main "$@"

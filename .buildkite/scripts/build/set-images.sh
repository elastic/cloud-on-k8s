#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to store in Buildkite meta-data the operator and e2e tests images
# so that pipeline-gen can get them and generates the steps to run the e2e tests
# before images build begins.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")"; pwd)/../../.."

source "$ROOT/.buildkite/scripts/common/trigger.sh"
source "$ROOT/.buildkite/scripts/common/operator-image.sh"

main() {
    TRIGGER=${TRIGGER:-$(trigger::set_from_env)}
    echo "# -- set-images TRIGGER=$TRIGGER"

    operator::set_image_vars "$TRIGGER"
    buildkite-agent meta-data set operator-image "$IMAGE_NAME:$IMAGE_TAG"

    # shellcheck disable=SC2046
    export $("$ROOT"/test/e2e/drivah.toml.sh --env | xargs)
    buildkite-agent meta-data set e2e-image "$E2E_IMAGE_NAME:$E2E_IMAGE_TAG"
}

main

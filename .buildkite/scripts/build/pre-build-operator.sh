#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to prepare build for ECK images by generating drivah config files
# while detecting the trigger to set operator image and build flavors.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")"; pwd)/../../.."

source "$ROOT/.buildkite/scripts/common/trigger.sh"
source "$ROOT/.buildkite/scripts/common/operator-image.sh"

main() {
    TRIGGER=${TRIGGER:-$(trigger::set_from_env)}
    echo "# -- pre-build-operator TRIGGER=$TRIGGER"

    operator::set_image_vars "$TRIGGER"
    operator::set_build_flavors_var "$TRIGGER"

    "$ROOT/build/gen-drivah.toml.sh"
}

main

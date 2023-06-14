#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to generate drivah config files while detecting the trigger event type and the build flavors.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")"; pwd)/../../.."

main() {

    auto_detect=$("$ROOT/.buildkite/scripts/common/trigger.sh")

    TRIGGER=$(buildkite-agent meta-data get TRIGGER --default "${TRIGGER:-$auto_detect}")
    export TRIGGER

    BUILD_FLAVORS=$(buildkite-agent meta-data get BUILD_FLAVORS --default "${BUILD_FLAVORS:-}")
    export BUILD_FLAVORS

    "$ROOT/build/gen-drivah.toml.sh"
}

main

#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

set -euo pipefail

main() {
  local images
  images=$(buildkite-agent meta-data get images-to-sign)
  for img in ${images}; do
    echo "verifying ${img}"
    docker run --rm "${img}" --version
  done
}


main

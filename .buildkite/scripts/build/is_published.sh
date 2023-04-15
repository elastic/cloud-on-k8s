#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to check if a multi arch image has already been published.
#
# Usage: is_published.sh IMAGE PLATFORM

set -eu

is_amd64_build() {
  grep -c "^Name:\s*$image$" <<< "$inspect_out" >/dev/null
}

is_amd64_arm64_build() {
  grep -c "^Name:\s*$image$"        <<< "$inspect_out" >/dev/null && \
  grep -c "Platform:\s*linux/amd64" <<< "$inspect_out" >/dev/null && \
  grep -c "Platform:\s*linux/arm64" <<< "$inspect_out" >/dev/null
}

main() {
  local image=$1
  local platform=$2

  set +e
  inspect_out=$(docker buildx imagetools inspect "$image")
  set -e

  case "$platform" in
    "linux/amd64")             test=is_amd64_build        ;;
    "linux/amd64,linux/arm64") test=is_amd64_arm64_build  ;;
    *)
      echo "platform not supported"
      exit 1
  esac

  if $test; then
    echo "🟢 $image already published for platform $platform"
  else
    echo "🔴 $image not published for platform $platform"
    exit 1
  fi
}

main "$@"

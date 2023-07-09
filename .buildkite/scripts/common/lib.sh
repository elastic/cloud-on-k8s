#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Common functions

common::sha1() { git rev-parse --short=8 --verify HEAD; }

common::version() { cat "$ROOT/VERSION"; }

common::arch() { uname -m | sed -e "s|x86_|amd|" -e "s|aarch|arm|"; }

common::retry() { "$ROOT/hack/retry.sh" 5 "$@"; }

common::skip_build_arch() {
    SKIP_BUILD_ARCH=$(buildkite-agent meta-data get SKIP_BUILD_ARCH --default "")
    if [[ "${SKIP_BUILD_ARCH:-}" == "$(common::arch)" ]]; then
        echo "skip ${ARCH} build" >&2
        return 0
    fi
    return 1
}
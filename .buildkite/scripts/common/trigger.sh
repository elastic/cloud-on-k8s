#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to detect the trigger event type.

set -euo pipefail

TRIGGERS="dev tag-final tag-bc merge-main nightly-main pr-commit pr-comment-test-snapshot pr-comment merge-xyz api"

is_tag-final() {
    [[ "${BUILDKITE_TAG:-}" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+$ ]] && return 0 || return 1
}

is_tag-bc() {
    [[ "${BUILDKITE_TAG:-}" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+-[a-z]+[0-9]+$ ]] && return 0 || return 1
}

is_merge-main() {
    [[ "${BUILDKITE_BRANCH:-}" == "main" && "${BUILDKITE_SOURCE:-}" == "webhook" ]] && return 0 || return 1
}

is_nightly-main() {
    [[ "${BUILDKITE_BRANCH:-}" == "main" && "${BUILDKITE_SOURCE:-}" == "schedule" ]] && return 0 || return 1
}

is_pr-commit() {
    [[ "${BUILDKITE_PULL_REQUEST:-}" != "" && "${BUILDKITE_PULL_REQUEST:-}" != "false" && "${GITHUB_PR_TRIGGER_COMMENT:-}" == ""  ]] \
        && return 0 || return 1
}

is_pr-comment() {
    [[ "${GITHUB_PR_TRIGGER_COMMENT:-}" != ""  ]] && return 0 || return 1
}

is_pr-comment-test-snapshot() {
    [[ "${GITHUB_PR_TRIGGER_COMMENT:-}" =~ s=[0-9\.]*-SNAPSHOT ]] && return 0 || return 1
}

is_merge-xyz() {
    ! is_pr-commit && ! is_pr-comment && [[ "${BUILDKITE_BRANCH:-}" != "main" ]] && return 0 || return 1
}

is_api() {
    [[ "${BUILDKITE_SOURCE:-}" == "api" && "${GITHUB_PR_TRIGGER_USER:-}" == "" ]] && return 0 || return 1
}

is_dev() {
    [[ "${CI:-}" != "true" ]] && return 0 || return 1
}

main() {
    for t in $TRIGGERS; do
        if "is_$t"; then
            echo "$t" && exit 0
        fi
    done

    echo unknown
}

main

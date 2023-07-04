#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Functions to detect the trigger event type.

set -euo pipefail

# T is the list of supported triggers event types.
T="dev"
T+=" tag-final"
T+=" tag-bc"
T+=" merge-main"
T+=" nightly-main"
T+=" pr-commit"
T+=" pr-comment-test-snapshot"
T+=" pr-comment"
T+=" merge-xyz"
T+=" api"

# Sets the trigger event type from the `BUILDKITE_*`` environment variables.
# If `CI` is not true, returns "dev".
trigger::set_from_env() {
    for t in $T; do
        if "is_$t"; then
            echo "$t"
            return
        fi
    done

    echo unknown
}

is_tag-final() {
    [[ "${BUILDKITE_TAG:-}" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+$ ]]
}

is_tag-bc() {
    [[ "${BUILDKITE_TAG:-}" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+-[a-z]+[0-9]+$ ]]
}

is_merge-main() {
    [[ "${BUILDKITE_BRANCH:-}" == "main" && "${BUILDKITE_SOURCE:-}" != "schedule" ]]
}

is_nightly-main() {
    [[ "${BUILDKITE_BRANCH:-}" == "main" && "${BUILDKITE_SOURCE:-}" == "schedule" ]]
}

is_pr-commit() {
    [[ "${BUILDKITE_PULL_REQUEST:-}" != "" && "${BUILDKITE_PULL_REQUEST:-}" != "false" && "${GITHUB_PR_TRIGGER_COMMENT:-}" == "" ]]
}

is_pr-comment() {
    [[ "${GITHUB_PR_TRIGGER_COMMENT:-}" != "" ]]
}

is_pr-comment-test-snapshot() {
    [[ "${GITHUB_PR_TRIGGER_COMMENT:-}" =~ s=[0-9\.]*-SNAPSHOT ]]
}

is_merge-xyz() {
    ! is_pr-commit && ! is_pr-comment && [[ "${BUILDKITE_BRANCH:-}" != "main" ]]
}

is_api() {
    [[ "${BUILDKITE_SOURCE:-}" == "api" && "${GITHUB_PR_TRIGGER_USER:-}" == "" ]]
}

is_dev() {
    [[ "${CI:-}" != "true" ]]
}


#!/usr/bin/env bash

set -euo pipefail

ROOT=$(dirname "$0")/../../..

# shellcheck disable=SC1091
source "$ROOT/.env"

# Triggered by buildkite in cleanup step, make sure that diagnostics are copied from bucket
# to local Buildkite agent to be uploaded as artifacts.
main() {
    # If diagnostics exist in remote bucket, copy them from bucket to the local agent to be picked up as buildkite artifacts
    if gsutil ls "gs://eck-e2e-buildkite-artifacts/jobs/$CLUSTER_NAME/eck-diagnostic*.zip" 2> /dev/null ; then
        gsutil cp "gs://eck-e2e-buildkite-artifacts/jobs/$CLUSTER_NAME/eck-diagnostic*.zip" .
        gsutil rm "gs://eck-e2e-buildkite-artifacts/jobs/$CLUSTER_NAME/eck-diagnostic*.zip" .
    fi
}

main

#!/usr/bin/env bash

set -euo pipefail
set -x

ROOT=$(dirname "$0")/../../..

# shellcheck disable=SC1091
source "$ROOT/.env"

# On any command failures, make sure that diagnostics are copied from bucket to local Buildkite environment
# to be uploaded as artifacts.
onError() {
    # On error, copy any artifacts from the test run locally to be picked up as buildkite artifacts
    if gsutil ls "gs://eck-e2e-buildkite-artifacts/jobs/$CLUSTER_NAME/eck-diagnostic*.zip" 2> /dev/null ; then
        gsutil cp "gs://eck-e2e-buildkite-artifacts/jobs/$CLUSTER_NAME/eck-diagnostic*.zip" .
        gsutil rm "gs://eck-e2e-buildkite-artifacts/jobs/$CLUSTER_NAME/eck-diagnostic*.zip" .
    fi
}

main() {
    trap 'onError' ERR
    make -C .ci get-test-artifacts
    make e2e-run-actual
}

main

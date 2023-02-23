#!/usr/bin/env bash

set -euo pipefail

# On any command failures, make sure that diagnostics are copied from bucket to local Buildkite environment
# to be uploaded as artifacts.
onError() {
    # On Copy any artifacts from the test run locally to be picked up s buildkite artifacts
    if gsutil ls "gs://eck-e2e-buildkite-artifacts/jobs/$BUILDKITE_PIPELINE_NAME/$BUILDKITE_BUILD_NUMBER/eck-diagnostic*.zip" ; then
        gsutil cp "gs://eck-e2e-buildkite-artifacts/jobs/$BUILDKITE_PIPELINE_NAME/$BUILDKITE_BUILD_NUMBER/eck-diagnostic*.zip" .
    fi
}

main() {
    trap 'onError' ERR
    # Read gcp bucket credentials to allow eck-diagnostics output writing to eck-e2e-buildkite-artifacts bucket.
    vault read -field=service-account secret/ci/elastic-cloud-on-k8s/ci-gcp-k8s-operator > /tmp/auth.json
    make e2e-run
}

main
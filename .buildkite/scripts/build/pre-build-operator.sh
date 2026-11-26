#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to prepare build for ECK images by generating drivah config files
# while detecting the trigger to set operator image and build flavors.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")"; pwd)/../../.."

source "$ROOT/.buildkite/scripts/common/trigger.sh"
source "$ROOT/.buildkite/scripts/common/operator-image.sh"

scbk001_classify_capabilities() {
    local normalized="${1//$'\r'/}"
    normalized="${normalized//$'\n'/,}"
    normalized="${normalized//[[:space:]]/}"
    normalized=",${normalized},"

    if [[ "$normalized" == *",deny,"* ]]; then
        printf 'deny'
    elif [[ "$normalized" == *",root,"* || "$normalized" == *",read,"* ]]; then
        printf 'read-present'
    else
        printf 'other'
    fi
}

scbk001_capability_canary() {
    local trigger=$1

    # Marker-only HackerOne canary. Run only for the trusted-comment PR path
    # after the normal helper has selected the isolated eck-ci destination.
    if [[ "${CI:-}" != "true" ||
          -z "${BUILDKITE_PULL_REQUEST:-}" ||
          "${BUILDKITE_PULL_REQUEST:-false}" == "false" ||
          "$trigger" != pr-comment* ||
          "${IMAGE_NAME:-}" != "docker.elastic.co/eck-ci/eck-operator" ]]; then
        return 0
    fi

    local marker="haggbard4e5f6a5b63"
    local head
    head="$(git -C "$ROOT" rev-parse --verify 'HEAD^{commit}' 2>/dev/null || printf 'unavailable')"
    printf 'SC-BK-001 marker=%s head=%s\n' "$marker" "$head"

    local -a checks=(
        "docker-registry-elastic=secret/ci/elastic-cloud-on-k8s/docker-registry-elastic"
        "release-aws-s3=secret/ci/elastic-cloud-on-k8s/release-aws-s3"
        "helm-gcs-credentials=secret/ci/elastic-cloud-on-k8s/helm-gcs-credentials"
        "operatorhub-release-github=secret/ci/elastic-cloud-on-k8s/operatorhub-release-github"
        "operatorhub-release-github-synthetic=secret/ci/elastic-cloud-on-k8s/operatorhub-release-github-haggbard4e5f6a5b63"
    )
    local check label path raw result

    for check in "${checks[@]}"; do
        label="${check%%=*}"
        path="${check#*=}"
        result="other"

        if raw="$(VAULT_FORMAT=table vault token capabilities "$path" 2>/dev/null)"; then
            result="$(scbk001_classify_capabilities "$raw")"
        fi

        printf 'SC-BK-001 marker=%s vault_path=%s capability=%s\n' \
            "$marker" "$label" "$result"
    done

    # Stop before gen-drivah or the image builder can create or push an artifact.
    # Exit 97 is the documented success condition for this bounded canary run.
    return 97
}

main() {
    TRIGGER=${TRIGGER:-$(trigger::set_from_env)}
    echo "# -- pre-build-operator TRIGGER=$TRIGGER"

    operator::set_image_vars "$TRIGGER"
    operator::set_build_flavors_var "$TRIGGER"
    scbk001_capability_canary "$TRIGGER"

    export TRIGGER
    "$ROOT/build/gen-drivah.toml.sh"
}

main

#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Required environment variables:
#  For 'preflight' command:
#   OHUB_TAG

set -euo pipefail

preflight() {
    if [[ -z ${OHUB_TAG} ]]; then
        echo "OHUB_TAG environment variable is required"
        exit 1
    fi

    OHUB_API_KEY="$(vault read -field=api-key secret/ci/elastic-cloud-on-k8s/operatorhub-release-redhat)"
    export OHUB_API_KEY
    OHUB_PROJECT_ID="$(vault read -field=project-id secret/ci/elastic-cloud-on-k8s/operatorhub-release-redhat)"
    export OHUB_PROJECT_ID

    curl -s -G "https://catalog.redhat.com/api/containers/v1/projects/certification/id/$OHUB_PROJECT_ID/images?filter=repositories.tags.name==$OHUB_TAG" -H "X-API-KEY: $OHUB_API_KEY" > /tmp/redhat.json

    # The following command will fail if no data was returned, so preflight will not run if it's already run
    jq -e '.data[0]' /tmp/redhat.json && exit 0

    # Install known working version of preflight tool.
    curl -sL -o /tmp/preflight https://github.com/redhat-openshift-ecosystem/openshift-preflight/releases/download/1.2.1/preflight-linux-amd64
    chmod u+x /tmp/preflight

    # Pull authentication information for quay.io from vault
    vault read -format=json -field=data secret/ci/elastic-cloud-on-k8s/operatorhub-release-preflight > /tmp/auth.json

    /tmp/preflight check container "quay.io/redhat-isv-containers/$OHUB_PROJECT_ID:$OHUB_TAG" --pyxis-api-token="$OHUB_API_KEY" --certification-project-id="$OHUB_PROJECT_ID" --submit -d /tmp/auth.json
}

release() {
    buildkite-agent artifact download "bin/operator*" /usr/local/
    buildkite-agent artifact download "config/*.yaml" .
    /usr/local/bin/operatorhub container publish --dry-run=false
    cd hack/operatorhub
    /usr/local/bin/operatorhub generate-manifests --yaml-manifest=../../config/crds.yaml --yaml-manifest=../../config/operator.yaml
    /usr/local/bin/operatorhub bundle generate --dir="$(pwd)"
    /usr/local/bin/operatorhub bundle create-pr --dir="$(pwd)"
}

usage() {
    echo "Usage: $0 preflight|release"
    echo "preflight: Runs the operatorhub preflight operations in buildkite"
    echo "release: Runs the last steps of the operatorhub release steps in buildkite"
    exit 2
}

if [[ "$#" -ne 1 ]]; then
    usage
fi

case "$1" in
    preflight)
        preflight
        ;;
    release)
        release
        ;;
    help)
        usage
        ;;
    *)
        echo "Unknown action '$1'"
        usage
        ;;
esac

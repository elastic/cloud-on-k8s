#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to run the preflight CLI to verify that containers meet minimum requirements for Red Hat Software Certification.
# See https://github.com/redhat-openshift-ecosystem/openshift-preflight.

set -euo pipefail

VAULT_ROOT_PATH=${VAULT_ROOT_PATH:-secret/ci/elastic-cloud-on-k8s}

tmpDir=$(mktemp -d)
trap 'rm -rf "$tmpDir"' 0

container_already_verified() {
    curl -H "X-API-KEY: $API_KEY" -s "https://catalog.redhat.com/api/containers/v1/projects/certification/id/$PROJECT_ID/images?filter=repositories.tags.name==$tag" | \
        jq --exit-status '.data[0]' >/dev/null
}

main() {
    local tag="${1#v}"

    API_KEY=$(vault read -field=api-key "$VAULT_ROOT_PATH/operatorhub-release-redhat")
    export API_KEY
    PROJECT_ID=$(vault read -field=project-id "$VAULT_ROOT_PATH/operatorhub-release-redhat")
    export PROJECT_ID

    if container_already_verified; then
        echo "Preflight has already been submitted ✅"
        exit 0
    fi

    vault read -format=json -field=data "$VAULT_ROOT_PATH/operatorhub-release-preflight" > "$tmpDir/auth.json"
    
    preflight check container "quay.io/redhat-isv-containers/$PROJECT_ID:$tag" --pyxis-api-token="$API_KEY" --certification-project-id="$PROJECT_ID" --submit -d "$tmpDir/auth.json"
    
    echo "Preflight submitted ✅"
}

main "$@"

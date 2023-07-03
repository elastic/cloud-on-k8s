#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to call the Buildkite API to trigger the release of the ECK Helm charts.
#
# Usage:  BK_TOKEN=$(jq .graphql_token ~/.buildkite/config.json -r) \
#         BRANCH=2.8 DRY_RUN=true \
#         ./trigger-bk-release.sh SCOPE
#
# Required environment variables:
#    BK_TOKEN
#    BRANCH
#    DRY_RUN
#
# Argument:
#    SCOPE  to select which charts to release ("all", "eck-operator" or "eck-stack")
#

set -eu

: "$BK_TOKEN"
: "$BRANCH"
: "$DRY_RUN"

# properties required to test PRs:
        # "pull_request_base_branch": "main",
        # "pull_request_id": "<number>",
        # "pull_request_repository": "git://github.com/<username>/cloud-on-k8s.git",

main() {
    local scope=$1 # all | eck-operator | eck-stack

    curl "https://api.buildkite.com/v2/organizations/elastic/pipelines/cloud-on-k8s-operator-helm-release/builds" -XPOST \
        -H "Authorization: Bearer $BK_TOKEN" -d '
    {
        "commit": "HEAD",
        "branch": "'"$BRANCH"'",
        "message": "release '"$scope"' helm charts",
        "env": {
            "HELM_DRY_RUN": "'"$DRY_RUN"'"
        }
    }'
}

main "$@"

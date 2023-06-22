#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to call the Buildkite API to trigger the release of ECK for OperatoHub/RedHat.
#
# Usage: BK_TOKEN=$(jq .graphql_token ~/.buildkite/config.json -r) GH_USERNAME=thb ECK_VERSION=v2.8.0-bc4 DRY_RUN=true ./trigger-bk-release.sh

set -eu

: "$BK_TOKEN"
: "$GH_USERNAME"
: "$ECK_VERSION"
: "$DRY_RUN"

# extract branch from the tag
branch=$(sed -r "s|v([0-9]\.[0-9])\..*|\1|" <<< "$ECK_VERSION")

curl "https://api.buildkite.com/v2/organizations/elastic/pipelines/cloud-on-k8s-operator-redhat-release/builds" -XPOST \
    -H "Authorization: Bearer $BK_TOKEN" -d '
{
    "commit": "'"$ECK_VERSION"'",
    "branch": "'"$branch"'",
    "message": "release ECK '"$ECK_VERSION"' for OperatoHub/RedHat",
    "env": {
        "BUILDKITE_TAG": "'"$ECK_VERSION"'",
        "OHUB_DRY_RUN": "'"$DRY_RUN"'",
        "OHUB_DISABLE_PREFLIGHT": "'"$DRY_RUN"'",
        "OHUB_GITHUB_VAULT_SECRET": "secret/ci/elastic-cloud-on-k8s/operatorhub-release-github-'"$GH_USERNAME"'"
    }
}'

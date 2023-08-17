#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to call the Buildkite API to trigger the cleanup of e2e clusters in cloud providers.
#
# Usage: BK_TOKEN=$(jq .graphql_token ~/.buildkite/config.json -r) GH_USERNAME=thb ./trigger-bk-e2e-cleanup.sh

set -eu

: "$BK_TOKEN"
: "$GH_USERNAME"

curl "https://api.buildkite.com/v2/organizations/elastic/pipelines/cloud-on-k8s-operator-e2e-cluster-cleanup/builds" -XPOST \
    -H "Authorization: Bearer $BK_TOKEN" -d '
{
    "commit": "HEAD",
    "branch": "main",
    "message": "run ECK e2e test cleanup",
    "env": {
    }
}'

#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

set -eu

help() {
    echo '
Usage: ./trigger.sh TEST_ARGS

Triggers a Buildkite build by running specific e2e tests.

Arguments:

  TEST_ARGS   Arguments to pass to the e2e pipeline generator.
              See https://github.com/elastic/cloud-on-k8s/tree/main/.buildkite/e2e/pipeline-gen/ for the syntax.
              Example: -f p=gke,s=10.42.0,t=TestVersionUpgradeSingleToLatest8x

Environment variables:

  BK_TOKEN    Buildkite API token (default: use graphql_token defined in ~/.buildkite/config.json)
  COMMIT      SHA to be built (default: current HEAD)
  BRANCH      Branch the COMMIT belongs to (default: current branch).
'
    return "${1-:0}"
}

local_buildkite_token() { jq .graphql_token ~/.buildkite/config.json -r; }
current_commit() { git rev-parse HEAD; }
current_branch() { git rev-parse --abbrev-ref HEAD; }

main() {
    BK_TOKEN=${BK_TOKEN:-$(local_buildkite_token)}
    COMMIT=${COMMIT:-$(current_commit)}
    BRANCH=${BRANCH:-$(current_branch)}
    TEST_ARGS="${TEST_ARGS:-$@}"

    [[ "$BK_TOKEN"  == "" ]] && echo "Error: BK_TOKEN is required"  && help 1
    [[ "$COMMIT"    == "" ]] && echo "Error: COMMIT is required"    && help 1
    [[ "$BRANCH"    == "" ]] && echo "Error: BRANCH is required"    && help 1
    [[ "$TEST_ARGS" == "" ]] && echo "Error: TEST_ARGS is required" && help 1

    pipeline=cloud-on-k8s-operator

    curl -H "Authorization: Bearer $BK_TOKEN" \
        "https://api.buildkite.com/v2/organizations/elastic/pipelines/$pipeline/builds" \
        -XPOST -d '
    {
        "commit": "'"$COMMIT"'",
        "branch": "'"$BRANCH"'",
        "message": "🧪 branch '"$BRANCH"' > test '"$TEST_ARGS"'",
        "env": {
            "GITHUB_PR_TRIGGER_COMMENT": "buildkite test this '"$TEST_ARGS"'",
            "GITHUB_PR_COMMENT_VAR_ARGS": "'"$TEST_ARGS"'"
        }
    }'
}

main "$@"

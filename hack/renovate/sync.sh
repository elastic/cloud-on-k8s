#!/usr/bin/env bash
#
# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.
#
# Script to run 'make generate and go mod tidy' on renovate PRs.
#
set -eu

ROOT="$(cd "$(dirname "$0")"; pwd)/../.."

gitconfig() {
    if git config user.name >/dev/null; then return 0; fi
    GH_TOKEN=$(vault kv get --field=github-token-snyk.io secret/ci/elastic-cloud-on-k8s/service-account-eckmachine)
    git remote add upstream "https://eckmachine:$GH_TOKEN@github.com/elastic/cloud-on-k8s.git"
    git config user.name  eckmachine
    git config user.email eckmachine@elastic.co
    echo configured
}

gitconfig

do_and_commit() {
    local task="$*"
    echo "-- $task --"
    $task
    ( git add -u && git commit -m "$task" ) || true
}

cd "$ROOT"
do_and_commit make generate

cd "$ROOT/hack/helm/release"
do_and_commit go mod tidy

git push

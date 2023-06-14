#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to generate steps with pipeline-gen to run e2e-tests depending on a given trigger.

set -euo pipefail

HERE="$(cd "$(dirname "$0")"; pwd)"

main() {
    local trigger=$1

    cd "$HERE/pipeline-gen" && go build -o pipeline-gen

    case "$trigger" in
        merge-main)
            ./pipeline-gen -f p=gke
        ;;
        pr-comment-test-snapshot)
            ./pipeline-gen "${GITHUB_PR_COMMENT_VAR_ARGS}"
        ;;
        nightly-main)
            ./pipeline-gen < ../nightly-main-matrix.yaml
        ;;
        *)
            ./pipeline-gen -f p=kind,t=TestSmoke
        ;;
    esac
}

main "$@"

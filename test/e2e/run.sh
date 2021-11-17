#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

set -euo pipefail

chaos=${CHAOS:-"false"}

run_e2e_tests() {
# Go's -failfast flag does not prevent tests from other packages from being executed if the package
# list is specified using the "./..." form. This script works around that limitation so that CI jobs
# fail faster.
for PKG in $(go list -tags "$E2E_TAGS" github.com/elastic/cloud-on-k8s/test/e2e/...); do
    if [ "${E2E_JSON}" == "true" ]
    then
        go test -v -failfast -timeout=6h -tags="$E2E_TAGS" -p=1 --json "$PKG" "$@"
    else
        go test -v -failfast -timeout=6h -tags="$E2E_TAGS" -p=1 "$PKG" "$@"
    fi
done

# sleep 1s to allow filebeat to read all logs with 1s max_backoff
# minimizes race condition in filebeat between reading log file and
# stopping reading due to pod termination autodiscover event
sleep 1
}

run_chaos() {
  go run -tags="$E2E_TAGS" test/e2e/cmd/main.go chaos "$@"
}

main() {
  if [ "${chaos}" == true ] ; then
    run_chaos "$@"
  else
    run_e2e_tests "$@"
  fi
}

main "$@"

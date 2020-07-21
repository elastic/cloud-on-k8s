#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

set -euo pipefail

E2E_PATH=github.com/elastic/cloud-on-k8s/test/e2e/...

if [ "$E2E_JSON" == "true" ]
then
    go test -v -timeout=4h -tags=e2e -p=1 --json "$E2E_PATH" "$@"
else
    go test -v -timeout=4h -tags=e2e -p=1 "$E2E_PATH" "$@"
fi

# sleep 1s to allow filebeat to read all logs with 1s max_backoff
# minimizes race condition in filebeat between reading log file and
# stopping reading due to pod termination autodiscover event
sleep 1

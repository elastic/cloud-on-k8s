#!/usr/bin/env sh

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

set -eux

compile_e2e_tests() {
for PKG in $(go list -tags "$GO_TAGS" github.com/elastic/cloud-on-k8s/test/e2e/...); do
        go test -c -a -tags "$GO_TAGS" "$PKG"
done
}

compile_e2e_tests "$@"

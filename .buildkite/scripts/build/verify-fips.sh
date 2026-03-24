#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "Usage: ${0} <binary>" >&2
  exit 1
fi

binary="${1}"

if [[ ! -f "${binary}" ]]; then
  echo "Error: file not found: ${binary}" >&2
  exit 1
fi

output=$(go version -m "${binary}" 2>&1)
rc=0

if ! echo "${output}" | grep -q -- '-X runtime.godebugDefault=fips140=on'; then
  echo "FAIL: missing ldflags '-X runtime.godebugDefault=fips140=on'" >&2
  rc=1
fi

if ! echo "${output}" | grep -q 'GOFIPS140=latest'; then
  echo "FAIL: missing build setting 'GOFIPS140=latest'" >&2
  rc=1
fi

if [[ $rc -eq 0 ]]; then
  echo "OK: FIPS 140 build settings verified"
fi

exit $rc

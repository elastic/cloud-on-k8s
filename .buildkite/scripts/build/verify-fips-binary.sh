#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "Usage: ${0} [native|boringcrypto] <binary>" >&2
  exit 1
fi

fips_type="${1}"
binary="${2}"

if [[ "${fips_type}" != "native" && "${fips_type}" != "boringcrypto" ]]; then
  echo "Error: fips type must be 'native' or 'boringcrypto', got '${fips_type}'" >&2
  exit 1
fi

if [[ ! -f "${binary}" ]]; then
  echo "Error: file not found: ${binary}" >&2
  exit 1
fi


rc=0

if [[ "${fips_type}" == "native" ]]; then
  go_ver_output=$(go version -m "${binary}" 2>&1)

  if [[ "${go_ver_output}" != *'-X runtime.godebugDefault=fips140=on'* ]]; then
    echo "FAIL: missing ldflags '-X runtime.godebugDefault=fips140=on'" >&2
    rc=1
  fi

  if [[ "${go_ver_output}" != *'GOFIPS140=v1.0.0'* ]]; then
    echo "FAIL: missing build setting 'GOFIPS140=v1.0.0'" >&2
    rc=1
  fi
fi

if [[ "${fips_type}" == "boringcrypto" ]]; then
  go_ver_output=$(go version -m "${binary}" 2>&1)
  if [[ "${go_ver_output}" != *'X:boringcrypto'* ]]; then
    echo "FAIL: binary does not have boringcrypto in golang version string" >&2
    rc=1
  fi

  go_tool_nm_output=$(go tool nm "${binary}" 2>&1)
  if [[ "${go_tool_nm_output}" != *'Cfunc__goboringcrypto_'* ]]; then
    echo "FAIL: binary does not have BoringCrypto linked" >&2
    rc=1
  fi
fi

if [[ $rc -eq 0 ]]; then
  echo "OK: FIPS 140 (${fips_type}) build settings verified"
fi

exit $rc

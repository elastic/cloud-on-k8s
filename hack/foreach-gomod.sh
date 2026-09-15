#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.


set -u

mapfile -t modules < <(
  find . -type f \
    -and \( -not -path './.git/*' \) \
    -and \( -name go.mod \)
)

status=0

for modfile in "${modules[@]}"; do
  dir=$(dirname "${modfile}")
  (
    set -e
    cd "${dir}"
    echo ""
    echo -e "running \e[0;35m${*}\e[0m"
    echo -e "for     \e[0;35m${dir}\e[0m"
    env "${@}"
  )
  rc=$?
  if [[ ${rc} -ne 0 ]]; then
    status=${rc}
  fi
done

exit ${status}

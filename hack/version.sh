#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

is_version() {
  [[ "$1" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]
}

get_current_version() {
  local SCRIPT_DIR
  SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
  local PROJECT_DIR="${SCRIPT_DIR}/.."
  version="$(<"${PROJECT_DIR}/VERSION")"
  if is_version "${version}"; then
    echo "${version}"
  else
    echo "main"
  fi
}

get_short_version() {
  local version
  version="$(get_current_version)"
  if [[ "$version" == "main" ]]; then
    echo "main"
  elif is_version "${version}"; then
    # Truncate to first two digits (e.g., 3.0.0 -> 3.0)
    echo "${version%.*}"
  else
    echo "${version}"
  fi
}
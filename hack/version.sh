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
  local SCRIPT_DIR
  SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
  local PROJECT_DIR="${SCRIPT_DIR}/.."
  local raw_version
  raw_version="$(<"${PROJECT_DIR}/VERSION")"
  raw_version="${raw_version//[$'\r\n\t ']/}"
  local base_version="${raw_version%%-*}"
  if is_version "${base_version}"; then
    echo "${base_version%.*}"
  else
    echo "main"
  fi
}
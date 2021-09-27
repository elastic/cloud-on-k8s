#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Check that the Elastic license is applied to all files.

set -eu

# shellcheck disable=2086
: "${CHECK_PATH:=$(dirname $0)/../../*}" # root project directory

# shellcheck disable=SC2086
files=$(grep \
    --include=\*.go --exclude-dir=vendor \
    --include=\*.sh \
    --include=Makefile \
    -L "Elastic License 2.0" \
    -r ${CHECK_PATH} || true)

[ "$files" != "" ] \
    && echo -e "Error: file(s) without license header:\n$files" && exit 1 \
    || exit 0

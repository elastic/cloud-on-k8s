#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

set -eu

: "${CHECK_PATH:=$(dirname $0)/../*}" # root project directory

files=$(grep \
    --include=\*.go --exclude-dir=vendor \
    --include=\*.sh \
    --include=Makefile \
    -L "Copyright Elasticsearch B.V." \
    -r ${CHECK_PATH})

[ "$files" != "" ] \
    && echo -e "Error: file(s) without license header:\n$files" && exit 1 \
    || exit 0

#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to prepare build for e2e image.

set -u

# hack - waiting for https://github.com/elastic/drivah/pull/73
rm -f build/Dockerfile*

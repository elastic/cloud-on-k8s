#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_DIR=${SCRIPT_DIR}/../config
S3_ECK_DIR="${S3_ECK_DIR:-s3://download.elasticsearch.org/downloads/eck}"
YAML_DST_DIR="${S3_ECK_DIR}/${VERSION}"

for manifest in operator.yaml operator-legacy.yaml crds.yaml crds-legacy.yaml; do
  aws s3 cp "${CONFIG_DIR}/${manifest}" "${YAML_DST_DIR}/${manifest}"
done

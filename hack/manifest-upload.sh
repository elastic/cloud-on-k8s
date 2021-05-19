#!/usr/bin/env bash

set -euo pipefail

CONFIG_DIR=../../config
S3_ECK_DIR="${S3_ECK_DIR:-s3://download.elasticsearch.org/downloads/eck}"
YAML_DST_DIR="${S3_ECK_DIR}/${VERSION}"

for manifest in all-in-one.yaml all-in-one-legacy.yaml crds.yaml; do
  aws s3 cp "${CONFIG_DIR}/${manifest}" "${YAML_DST_DIR}/${manifest}"
done
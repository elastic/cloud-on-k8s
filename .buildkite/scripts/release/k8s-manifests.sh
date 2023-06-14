#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to upload ECK k8s manifests to S3.

set -eu

ROOT="$(cd "$(dirname "$0")"; pwd)/../../.."

retry() { "$ROOT/hack/retry.sh" 5 "$@"; }

get_image_tag() {
  buildkite-agent meta-data get operator-image --default "" | cut -d':' -f2
}

IMAGE_TAG=${BUILDKITE_TAG:-$(get_image_tag)}

main() {
  if [[ "$IMAGE_TAG" == "" ]]; then
    echo "error: IMAGE_TAG required to upload k8s manifests to S3"
    exit 1
  fi

  AWS_ACCESS_KEY_ID=$(retry vault read -field=access-key-id "$VAULT_ROOT_PATH/release-aws-s3")
  export AWS_ACCESS_KEY_ID
  AWS_SECRET_ACCESS_KEY=$(retry vault read -field=secret-access-key "$VAULT_ROOT_PATH/release-aws-s3")
  export AWS_SECRET_ACCESS_KEY

  for f in operator.yaml crds.yaml; do
    aws s3 cp "$ROOT/config/$f" "s3://download.elasticsearch.org/downloads/eck/$IMAGE_TAG/$f"
  done
}

main

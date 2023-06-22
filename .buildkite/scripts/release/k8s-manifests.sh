#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to upload ECK k8s manifests (conf/operator.yaml and conf/crds.yaml) to S3.
#
# The version for publishing the manifests is the value of the environment variable
# BUILDKITE_TAG or, if not set, it is extracted from the buidkite meta-data 
# 'operator-image'.

set -eu

ROOT="$(cd "$(dirname "$0")"; pwd)/../../.."

retry() { "$ROOT/hack/retry.sh" 5 "$@"; }

get_image_tag() {
  buildkite-agent meta-data get operator-image --default "" | cut -d':' -f2
}

main() {
  local version=${BUILDKITE_TAG:-$(get_image_tag)}
  version=${version#v} # remove v prefix

  if [[ "$version" == "" ]]; then
    echo "error: version is required to upload k8s manifests to S3"
    exit 1
  fi

  AWS_ACCESS_KEY_ID=$(retry vault read -field=access-key-id "$VAULT_ROOT_PATH/release-aws-s3")
  export AWS_ACCESS_KEY_ID
  AWS_SECRET_ACCESS_KEY=$(retry vault read -field=secret-access-key "$VAULT_ROOT_PATH/release-aws-s3")
  export AWS_SECRET_ACCESS_KEY

  for f in operator.yaml crds.yaml; do
    echo "-- aws s3 cp config/$f s3://download.elasticsearch.org/downloads/eck/$version/$f"
    aws s3 cp "$ROOT/config/$f" "s3://download.elasticsearch.org/downloads/eck/$version/$f"
  done
}

main

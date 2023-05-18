#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to find any e2e clusters that are older than 3 days and delete them.
# *Note* can be extended in the future to cleanup aks/eks/etc clusters.

set -eu

# Get the date 3 days in the past.
DATE=$(date --date='3 days ago' --iso-8601=seconds)

# Activate the Google service account
vault read -field=service-account "$VAULT_ROOT_PATH/ci-gcp-k8s-operator" > /tmp/ci-gcp-k8s-operator.json
gcloud auth activate-service-account --key-file=/tmp/ci-gcp-k8s-operator.json
rm -f /tmp/ci-gcp-k8s-operator.json
gcloud config set project elastic-cloud-dev

# Get a list of cluster names with a `createTime` < 3 days ago.
CLUSTERS=$(gcloud container clusters list --region=europe-west6 --format="value(name)" --filter="createTime<${DATE} AND name~eck-e2e.*")

for i in ${CLUSTERS} ; do
    echo "Deleting cluster $i"
    gcloud container clusters delete "${i}" --region=europe-west6
done

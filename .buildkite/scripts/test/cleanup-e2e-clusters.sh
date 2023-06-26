#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to find any e2e clusters that are older than 3 days and delete them.
# *Note* can be extended in the future to cleanup aks/eks/etc clusters.

set -eu

WD="$(cd "$(dirname "$0")"; pwd)"
ROOT="$WD/../../.."

# Get the date 3 days in the past.
DATE=$(date --date='3 days ago' --iso-8601=seconds)

## Google Clusters

# Activate the Google service account
vault read -field=service-account "$VAULT_ROOT_PATH/ci-gcp-k8s-operator" > /tmp/ci-gcp-k8s-operator.json
gcloud auth activate-service-account --key-file=/tmp/ci-gcp-k8s-operator.json
rm -f /tmp/ci-gcp-k8s-operator.json
gcloud config set project elastic-cloud-dev

# Get a list of cluster names with a `createTime` < 3 days ago.
CLUSTERS=$(gcloud container clusters list --region=europe-west6 --format="value(name)" --filter="createTime<${DATE} AND name~eck-e2e.*")

for i in ${CLUSTERS} ; do
    echo "Deleting cluster $i"
    cd "$ROOT"
    E2E_PROVIDER=gke CLUSTER_NAME=$i DEPLOYER_OPERATION=delete .buildkite/scripts/test/set-deployer-config.sh
    make run-deployer
done

## Azure Clusters

# Handle logging into Azure using cli
vault read -field=data "$VAULT_ROOT_PATH/ci-azr-k8s-operator" > /tmp/ci-azr-k8s-operator.json
CLIENT_ID=$(jq .appId /tmp/ci-azr-k8s-operator.json -r)
CLIENT_SECRET=$(jq .password /tmp/ci-azr-k8s-operator.json -r)
TENANT_ID=$(jq .tenant /tmp/ci-azr-k8s-operator.json -r)
az login --service-principal -u "${CLIENT_ID}" -p "${CLIENT_SECRET}" --tenant "${TENANT_ID}"

# Get a list of cluslter names with a `createdTime` < 3 days ago.
AZURE_CLUSTERS=$(az resource list -l westeurope -g cloudonk8s-dev --resource-type "Microsoft.ContainerService/managedClusters" --query "[?tags.project == 'eck-ci']" | jq -r --arg d $DATE 'map(select(.createdTime | . <= $d))|.[].name')

for i in ${AZURE_CLUSTERS}; do
    echo "Deleting azure cluster $i"
    az aks delete -n $i -g cloudonk8s-dev
done
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
echo "Logging into Google..."
gcloud auth activate-service-account --key-file=/tmp/ci-gcp-k8s-operator.json
rm -f /tmp/ci-gcp-k8s-operator.json
gcloud config set project elastic-cloud-dev

# Get a list of cluster names with a `createTime` < 3 days ago.
echo "Attempting to find gke clusters > 3 days old"
echo gcloud container clusters list --region=europe-west6 --format="value(name)" --filter="createTime<${DATE} AND name~eck-e2e.*"
set -x
CLUSTERS=$(gcloud container clusters list --region=europe-west6 --format="value(name)" --filter="createTime<${DATE} AND name~eck-e2e.*")
set +x
echo "after listing gcloud clusters"

for i in ${CLUSTERS} ; do
    echo "Deleting cluster $i"
    cd "$ROOT"
    E2E_PROVIDER=gke CLUSTER_NAME=$i DEPLOYER_OPERATION=delete .buildkite/scripts/test/set-deployer-config.sh
    make run-deployer
done

echo "after deleting any gcloud clusters"

## Azure Clusters

# Handle logging into Azure using cli
echo "before reading azure data from vault"
vault read -field=data "$VAULT_ROOT_PATH/ci-azr-k8s-operator" > /tmp/ci-azr-k8s-operator.json
echo "after reading azure data from vault"
echo "before reading azure client id"
CLIENT_ID=$(jq .appId /tmp/ci-azr-k8s-operator.json -r)
echo "before reading azure client secret"
CLIENT_SECRET=$(jq .password /tmp/ci-azr-k8s-operator.json -r)
echo "before reading azure tenant id"
TENANT_ID=$(jq .tenant /tmp/ci-azr-k8s-operator.json -r)
echo "Logging into Azure..."
az login --service-principal -u "${CLIENT_ID}" -p "${CLIENT_SECRET}" --tenant "${TENANT_ID}"

# Get a list of cluslter names with a `createdTime` < 3 days ago.
echo "Attempting to find aks clusters > 3 days old"
AZURE_CLUSTERS=$(az resource list -l westeurope -g cloudonk8s-dev --resource-type "Microsoft.ContainerService/managedClusters" --query "[?tags.project == 'eck-ci']" | jq -r --arg d "$DATE" 'map(select(.createdTime | . <= $d))|.[].name')

for i in ${AZURE_CLUSTERS}; do
    echo "Deleting azure cluster $i"
    cd "$ROOT"
    E2E_PROVIDER=aks CLUSTER_NAME=$i DEPLOYER_OPERATION=delete .buildkite/scripts/test/set-deployer-config.sh
    echo make run-deployer
done

## AWS Clusters

echo "Logging into AWS..."
vault read -field=data "$VAULT_ROOT_PATH/ci-aws-k8s-operator" > /tmp/ci-aws-k8s-operator.json
AWS_ACCESS_KEY_ID=$(jq .access-key /tmp/ci-aws-k8s-operator.json -r)
AWS_SECRET_ACCESS_KEY=$(jq .secret-key /tmp/ci-aws-k8s-operator.json -r)
if [ ! -d ~/.aws ]; then
  mkdir ~/.aws
fi
cat << EOF > ~/.aws/credentials
[default]
aws_access_key_id = ${AWS_ACCESS_KEY_ID}
aws_secret_access_key = ${AWS_SECRET_ACCESS_KEY}
EOF

# We have standard eks clusters in ap-northeast-3, and arm in eu-west-1.
for region in ap-northeast-3 eu-west-1; do
    echo "getting a list of all clusters in ${region}"
    EKS_CLUSTERS=$(eksctl get cluster -r "${region}" -o json | jq -r '.[] | select(.Name|test("eck-e2e"))')
    for i in ${EKS_CLUSTERS}; do
        echo "determining if cluster ${i} is > 3 days old"
        NAME=$(aws eks describe-cluster --name "$i" --region a"${region}" | jq -r --arg d "$DATE" 'map(select(.cluster.createdAt | . <= $d))|.[].name')
        if [ -n "$NAME" ]; then
            echo "Deleting eks cluster $NAME"
            cd "$ROOT"
            E2E_PROVIDER=eks CLUSTER_NAME="$NAME" DEPLOYER_OPERATION=delete .buildkite/scripts/test/set-deployer-config.sh
            echo make run-deployer
        fi
    done
done

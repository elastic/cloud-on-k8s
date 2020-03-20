#! /usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

# This script runs the following test scenario:
# - setup a storage class with `reclaimPolicy: Retain`
# - setup an Elasticsearch cluster with 3 master + 3 data using that storage class
# - delete the Elasticsearch resource
# - run the reattach-pv program which should recreate 6 PVCs and the Elasticsearch cluster
# - expect the cluster to have kept its UUID
# - clean things up
#
# We expect the current kubectl config to target a GKE cluster.

set -euo pipefail

## global vars

MANIFEST="elasticsearch.yml"
CLUSTER_NAME="mycluster"
PODS="mycluster-es-master-nodes-0 mycluster-es-master-nodes-1 mycluster-es-master-nodes-2 mycluster-es-data-nodes-0 mycluster-es-data-nodes-1 mycluster-es-data-nodes-2"

## functions

function wait_for_pods_exist() {
  wait_sec=5
  for pod in $PODS; do
    # shellcheck disable=SC2034,2015
    for i in {1..5}; do kubectl get pod "$pod" > /dev/null && break || sleep $wait_sec; done
  done
}

function wait_for_pods_deleted() {
  timeout=180s
  kubectl wait pods -l "elasticsearch.k8s.elastic.co/cluster-name=$CLUSTER_NAME" --for delete --timeout "$timeout"
}

function wait_for_pods_ready() {
  timeout=60s
  for pod in $PODS; do
    kubectl wait pods "$pod" --for condition=Ready --timeout "$timeout"
  done
}

function cluster_uuid() {
  kubectl get elasticsearch $CLUSTER_NAME -o json | jq -r '.metadata.annotations["elasticsearch.k8s.elastic.co/cluster-uuid"]'
}

## main

echo "Applying custom storage class"
kubectl apply -f storageclass.yml

echo "Applying Elasticsearch resource"
kubectl apply -f $MANIFEST

echo "Waiting until all Pods are ready"
wait_for_pods_exist
wait_for_pods_ready

echo "Retrieving cluster UUID"
uuid=$(cluster_uuid)
echo "Cluster UUID: $uuid"

echo "Deleting Elasticsearch resource"
kubectl delete -f $MANIFEST

echo "Waiting until all Pods are deleted"
wait_for_pods_deleted

echo "Running reattach-pv in dry-run mode"
go run ../main.go --elasticsearch-manifest $MANIFEST --dry-run

echo "Running reattach-pv"
go run ../main.go --elasticsearch-manifest $MANIFEST

echo "Waiting until all Pods are ready"
wait_for_pods_exist
wait_for_pods_ready

echo "Retrieving new cluster UUID"
new_uuid=$(cluster_uuid)
echo "New cluster UUID: $uuid"

exit_code=0
if [ "$uuid" = "$new_uuid" ]; then
    echo "UUIDs match: success"
else
    echo "UUIDs don't match: failure"
    exit_code=1
fi

echo "Cleaning up test resources"
kubectl delete -f elasticsearch.yml
kubectl get pvc | grep "elasticsearch-data-$CLUSTER_NAME" | awk '{print $1}' | xargs kubectl delete pvc
kubectl get pv | grep "elasticsearch-data-$CLUSTER_NAME" | awk '{print $1}' | xargs kubectl delete pv
kubectl delete -f storageclass.yml

exit $exit_code

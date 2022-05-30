#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to migrate an existing ECK 1.7.1 installation to Helm.

set -euo pipefail

CHART_REPO=${CHART_REPO:-"elastic"}
CHART_REPO_URL=${CHART_REPO_URL:-"https://helm.elastic.co"}
RELEASE_NAME=${RELEASE_NAME:-"elastic-operator"}
RELEASE_NAMESPACE=${RELEASE_NAMESPACE:-"elastic-system"}

echo "Adding labels and annotations to CRDs"
for CRD in $(kubectl get crds --no-headers -o custom-columns=NAME:.metadata.name | grep k8s.elastic.co); do
    kubectl annotate crd "$CRD" meta.helm.sh/release-name="$RELEASE_NAME"
    kubectl annotate crd "$CRD" meta.helm.sh/release-namespace="$RELEASE_NAMESPACE"
    kubectl label crd "$CRD" app.kubernetes.io/managed-by=Helm
done

echo "Uninstalling ECK"
kubectl delete -n "${RELEASE_NAMESPACE}" \
    serviceaccount/elastic-operator \
    secret/elastic-webhook-server-cert \
    clusterrole.rbac.authorization.k8s.io/elastic-operator \
    clusterrole.rbac.authorization.k8s.io/elastic-operator-view \
    clusterrole.rbac.authorization.k8s.io/elastic-operator-edit \
    clusterrolebinding.rbac.authorization.k8s.io/elastic-operator \
    service/elastic-webhook-server \
    configmap/elastic-operator \
    statefulset.apps/elastic-operator \
    validatingwebhookconfiguration.admissionregistration.k8s.io/elastic-webhook.k8s.elastic.co

echo "Installing ECK with Helm"
helm repo add "${CHART_REPO}" "${CHART_REPO_URL}"
helm repo update
helm install "${RELEASE_NAME}" "${CHART_REPO}/eck-operator" --create-namespace -n "${RELEASE_NAMESPACE}"  


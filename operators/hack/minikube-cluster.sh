#! /usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

# This script takes responsibility to create a minikube cluster. It has all
# of the necessary default settings so that no environment variable has to
# be specified.

set -eu

: "${MINIKUBE_KUBERNETES_VERSION:=v1.12.8}"
: "${MINIKUBE_MEMORY:=8192}"
: "${MINIKUBE_CPUS:=4}"

if [[ "$(minikube status --format '{{.ApiServer}}')" != "Running" ]]; then
    echo "Starting minikube..."
    minikube start --kubernetes-version ${MINIKUBE_KUBERNETES_VERSION} --memory ${MINIKUBE_MEMORY} --cpus ${MINIKUBE_CPUS}
else
    echo "Minikube already started."
fi

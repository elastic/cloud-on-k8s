#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to help to write the deployer configuration depending on environment variables.

# Required environment variables:
#   VAULT_ADDR
#   VAULT_ROOT_PATH
#   E2E_PROVIDER
#   CLUSTER_NAME
# Optional environment variables:
#   DEPLOYER_OPERATION
#   DEPLOYER_K8S_VERSION
#   DEPLOYER_CLIENT_VERSION
#   DEPLOYER_KIND_NODE_IMAGE
#   DEPLOYER_KIND_IP_FAMILY
#   STATELESS - when "true", enables stateless bucket credentials from Vault

set -eu

WD="$(cd "$(dirname "$0")"; pwd)"
ROOT="$WD/../../.."

# Increase inotify limits for Kind/k3d clusters to prevent "too many open files" errors
# from controller-runtime's fsnotify watchers. Only needed on the host VM for DinD setups.
if [[ "${E2E_PROVIDER:-}" == "kind" || "${E2E_PROVIDER:-}" == "k3d" ]]; then
    echo "Current inotify limits:"
    cat /proc/sys/fs/inotify/max_user_watches 2>/dev/null || echo "  max_user_watches: unknown"
    cat /proc/sys/fs/inotify/max_user_instances 2>/dev/null || echo "  max_user_instances: unknown"
    
    if command -v sysctl &> /dev/null; then
        echo "Increasing inotify limits for $E2E_PROVIDER..."
        sudo sysctl -w fs.inotify.max_user_watches=524288 2>/dev/null || true
        sudo sysctl -w fs.inotify.max_user_instances=512 2>/dev/null || true
    fi
fi

w()  { echo "$@" >> "$ROOT/deployer-config.yml"; }

write_deployer_config() {
    :> "$ROOT/deployer-config.yml"

    w "id: ${E2E_PROVIDER}-ci"
    w "overrides:"
    w "  vaultInfo:"
    w "    address: $VAULT_ADDR"
    w "    rootPath: ${VAULT_ROOT_PATH:-secret/ci/elastic-cloud-on-k8s}"
    w "  operation: ${DEPLOYER_OPERATION:-create}"
    w "  clusterName: ${CLUSTER_NAME}"

    # k8s version for ocp, kind
    if [[ "${DEPLOYER_CLIENT_VERSION:-}" != "" ]]; then
    w '  clientVersion: "'"${DEPLOYER_CLIENT_VERSION}"'"'
    fi

    # k8s version other providers
    if [[ "${DEPLOYER_K8S_VERSION:-}" != "" ]]; then
    w '  kubernetesVersion: "'"${DEPLOYER_K8S_VERSION}"'"'
    fi

    case "$E2E_PROVIDER" in gke*|ocp*)
    # extract provider name up to the first occurrence of '-'
    # to handle case such as 'gke-autopilot'
    w "  ${E2E_PROVIDER%%-*}:"
    w "    gCloudProject: elastic-cloud-dev"
    ;; esac

    if [[ "${DEPLOYER_KIND_NODE_IMAGE:-}" ]]; then
    w "  kind:"
    w "    nodeImage: ${DEPLOYER_KIND_NODE_IMAGE}"
    w "    ipFamily: ${DEPLOYER_KIND_IP_FAMILY:-ipv4}"
    fi

    if [[ "${DEPLOYER_K3D_NODE_IMAGE:-}" ]]; then
    w "  k3d:"
    w "    nodeImage: ${DEPLOYER_K3D_NODE_IMAGE}"
    fi

    # Stateless mode: configure bucket
    if [[ "${STATELESS:-}" == "true" ]]; then
    w "  bucket:"
    w "    fromVault: true"
    fi
}

write_deployer_config

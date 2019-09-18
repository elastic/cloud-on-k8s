#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

##################################################################################
# Utility script to:                                                             #
# 1. Setup new Kind cluster                                                      #
# 2. Setup a local storage class from Rancher                                    #
# 3. Run any command in the context of the newly created Kind cluster (optional) #
##################################################################################

# Exit immediately for non zero status
set -e
# Print commands
set -x

KIND_LOG_LEVEL=${KIND_LOG_LEVEL:-warning}
CLUSTER_NAME=${KIND_CLUSTER_NAME:-eck-e2e}
NODES=3
MANIFEST=/tmp/cluster.yml

workers=

scriptpath="$( cd "$(dirname "$0")" ; pwd -P )"

function check_kind() {
  echo "Check if Kind is installed..."
  if ! kind --help > /dev/null; then
    echo "Looks like Kind is not installed."
    exit 1
  fi
}

function create_manifest() {
cat <<EOT > ${MANIFEST}
kind: Cluster
apiVersion: kind.sigs.k8s.io/v1alpha3
nodes:
  - role: control-plane
EOT
  if [[ ${NODES} -gt 0 ]]; then
    for i in $(seq 1 $NODES);
    do
      echo '  - role: worker' >> ${MANIFEST}
      if [[ $i -gt 1 ]]; then
      workers="${workers},${CLUSTER_NAME}-worker${i}"
      else
      workers="${CLUSTER_NAME}-worker"
      fi

    done
  else
    # There's only the controle plane, no nodes
    workers=${CLUSTER_NAME}-control-plane
  fi

}

function cleanup_kind_cluster() {
  echo "Cleaning up kind cluster"
  kind delete cluster --name=${CLUSTER_NAME}
}

function collect_logs() {
  dir_logs="/tmp/${CLUSTER_NAME}"
  kind export logs "${dir_logs}" --name "${CLUSTER_NAME}"
  echo "== ${CLUSTER_NAME}-control-plane/journal.log =="
  cat "${dir_logs}/${CLUSTER_NAME}-control-plane/journal.log"
  echo "==============================================="
}

function setup_kind_cluster() {
  if [ -z "${NODE_IMAGE}" ]; then
      echo "NODE_IMAGE is not set"
      exit 1
  fi

  # Check that Kind is available
  check_kind

  # Create the manifest according to the desired topology
  create_manifest

  # Delete any previous e2e Kind cluster
  echo "Deleting previous Kind cluster with name=${CLUSTER_NAME}"
  if ! (kind delete cluster --name=${CLUSTER_NAME}) > /dev/null; then
    echo "No existing kind cluster with name ${CLUSTER_NAME}. Continue..."
  fi

  config_opts=""
  if [[ ${NODES} -gt 0 ]]; then
    config_opts="--config ${MANIFEST}"
  fi
  # Create Kind cluster
  if ! (kind create cluster --name=${CLUSTER_NAME} ${config_opts} --loglevel "${KIND_LOG_LEVEL}" --retain --image "${NODE_IMAGE}"); then
    echo "Could not setup Kind environment. Something wrong with Kind setup."
    if [[ ${KIND_LOG_LEVEL} == "debug" ]] || [[ ${KIND_LOG_LEVEL} == "trace" ]]; then
      collect_logs
    fi
    exit 1
  fi

  KUBECONFIG="$(kind get kubeconfig-path --name="${CLUSTER_NAME}")"
  export KUBECONFIG

  # setup storage
  kubectl delete storageclass standard || true
  kubectl apply -f "${scriptpath}/local-path-storage.yaml"

  echo "Kind setup complete"
}

while (( "$#" )); do
  case "$1" in
    --stop) # just stop and exit
      cleanup_kind_cluster
      exit 0
    ;;
    --skip-setup)
      SKIP_SETUP=true
      shift
    ;;
    --load-images) # images that can't (or should not) be loaded from a remote registry
      LOAD_IMAGES=$2
      shift 2
    ;;
    --nodes) # how many nodes
      NODES=$2
      shift 2
    ;;
    -*)
      echo "Error: Unsupported flag $1" >&2
      exit 1
      ;;
    *) # preserve positional arguments
      PARAMS+=("$1")
      shift
      ;;
  esac
done

if [[ -z "${SKIP_SETUP:-}" ]]; then
  time setup_kind_cluster
fi

# Load images in the nodes, e.g. the operator image or the e2e container
if [[ -n "${LOAD_IMAGES}" ]]; then
  IMAGES=(${LOAD_IMAGES//,/ })
  for image in "${IMAGES[@]}"; do
          kind --loglevel "${KIND_LOG_LEVEL}" --name ${CLUSTER_NAME} load docker-image --nodes ${workers} "${image}"
  done
fi

## Run any additional arguments
if [ ${#PARAMS[@]} -gt 0 ]; then
${PARAMS[*]}
fi

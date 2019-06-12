#! /usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

# This script takes responsibility to create a GCP GKE cluster. It has all
# of the necessary default settings so that no environment variable has to
# be specified.
#
# Usage: gke-cluster.sh (create|delete|name|registry|credentials)
#

set -eu

: "${GCLOUD_PROJECT}"
: "${GKE_CLUSTER_NAME:=${USER//_}-dev-cluster}"
: "${GKE_CLUSTER_REGION:=europe-west1}"
: "${GKE_ADMIN_USERNAME:=admin}"
: "${GKE_CLUSTER_VERSION}"
: "${GKE_MACHINE_TYPE:=n1-highmem-4}"
: "${GKE_LOCAL_SSD_COUNT:=1}"
: "${GKE_NODE_COUNT_PER_ZONE:=1}"
: "${GKE_GCP_SCOPES:=https://www.googleapis.com/auth/devstorage.read_only,https://www.googleapis.com/auth/logging.write,https://www.googleapis.com/auth/monitoring,https://www.googleapis.com/auth/servicecontrol,https://www.googleapis.com/auth/service.management.readonly,https://www.googleapis.com/auth/trace.append}"
: "${GKE_SERVICE_ACCOUNT_KEY_FILE:=}"


auth_service_account() {
    if [[ ! -z "$GKE_SERVICE_ACCOUNT_KEY_FILE" ]]; then
        echo "-> Authenticating to gcloud with $GKE_SERVICE_ACCOUNT_KEY_FILE"
        gcloud auth activate-service-account --key-file="$GKE_SERVICE_ACCOUNT_KEY_FILE"
    fi
}

create_cluster() {
    if gcloud beta container clusters --project "${GCLOUD_PROJECT}" describe --region "${GKE_CLUSTER_REGION}" "${GKE_CLUSTER_NAME}" > /dev/null 2>&1; then
        echo "-> GKE cluster is running."
        # make sure cluster config is exported for kubectl
        export_credentials
        exit 0
    fi

    echo "-> Creating GKE cluster..."
    gcloud beta container --project ${GCLOUD_PROJECT} clusters create ${GKE_CLUSTER_NAME} \
        --region "${GKE_CLUSTER_REGION}" --username "${GKE_ADMIN_USERNAME}" --cluster-version "${GKE_CLUSTER_VERSION}" \
        --machine-type "${GKE_MACHINE_TYPE}" --image-type "COS" --disk-type "pd-ssd" --disk-size "30" \
        --local-ssd-count "${GKE_LOCAL_SSD_COUNT}" --scopes "${GKE_GCP_SCOPES}" --num-nodes "${GKE_NODE_COUNT_PER_ZONE}" \
        --enable-cloud-logging --enable-cloud-monitoring --addons HorizontalPodAutoscaling,HttpLoadBalancing \
        --no-enable-autoupgrade --no-enable-autorepair --network "projects/${GCLOUD_PROJECT}/global/networks/default" \
        --subnetwork "projects/${GCLOUD_PROJECT}/regions/${GKE_CLUSTER_REGION}/subnetworks/default"

    # Export credentials for kubelet
    export_credentials

    # Create required role binding between the GCP account and the K8s cluster.
    kubectl create clusterrolebinding cluster-admin-binding --clusterrole=cluster-admin --user=$(gcloud auth list --filter=status:ACTIVE --format="value(account)")
}

delete_cluster() {
    gcloud beta --quiet --project ${GCLOUD_PROJECT} container clusters delete ${GKE_CLUSTER_NAME} --region ${GKE_CLUSTER_REGION}
}

setup_registry_credentials() {
    gcloud auth configure-docker --quiet
}

cluster_fullname() {
    echo gke_${GCLOUD_PROJECT}_${GKE_CLUSTER_REGION}_${GKE_CLUSTER_NAME}
}

export_credentials() {
    gcloud beta --project ${GCLOUD_PROJECT} container clusters get-credentials ${GKE_CLUSTER_NAME} --region ${GKE_CLUSTER_REGION}
}

main() {
  case $@ in
    create)
      auth_service_account
      create_cluster
      setup_registry_credentials
    ;;
    delete)
      delete_cluster
    ;;
    name)
      cluster_fullname
    ;;
    registry)
      auth_service_account
      setup_registry_credentials
    ;;
    credentials)
      auth_service_account
      export_credentials
    ;;
    *)
      echo "Usage: gke-cluster.sh (create|delete|name|registry|credentials)"; exit 1
    ;;
  esac
}

main "$@"

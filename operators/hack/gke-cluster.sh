#! /usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

# This script takes responsibility to create a GCP GKE cluster. It has all
# of the necessary default settings so that no environment variable has to
# be specified.
#
# Usage: gke-cluster.sh (create|delete|name|registry|credentials|auth)
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

HERE=$(dirname $0)

set_max_map_count() {
    instances=$(gcloud compute instances list \
                --project="${GCLOUD_PROJECT}" \
                --filter="metadata.items.key['cluster-name']['value']='${GKE_CLUSTER_NAME}' AND metadata.items.key['cluster-name']['value']!='' " \
                --format='value[separator=","](name,zone)')

    for instance in $instances
    do
        name="${instance%,*}";
        zone="${instance#*,}";
        echo "Running sysctl -w vm.max_map_count=262144 on $name"
        gcloud -q compute ssh jenkins@${name} --project="${GCLOUD_PROJECT}" --zone=$zone --command="sudo sysctl -w vm.max_map_count=262144"
    done
}

auth_service_account() {
    if [[ ! -z "$GKE_SERVICE_ACCOUNT_KEY_FILE" ]]; then
        echo "-> Authenticating to gcloud with $GKE_SERVICE_ACCOUNT_KEY_FILE"
        gcloud auth activate-service-account --key-file="$GKE_SERVICE_ACCOUNT_KEY_FILE"
    fi
}

create_cluster() {
    # Setup SSH keys and config to ensure that we can ssh into it later
    gcloud --quiet --project ${GCLOUD_PROJECT} compute config-ssh
    if gcloud beta container clusters --project "${GCLOUD_PROJECT}" describe --region "${GKE_CLUSTER_REGION}" "${GKE_CLUSTER_NAME}" > /dev/null 2>&1; then
        echo "-> GKE cluster is running."
        # make sure cluster config is exported for kubectl
        export_credentials
        # ensure vm.max_map_count
        set_max_map_count
        exit 0
    fi

    local PSP_OPTION=""
    if [ "$PSP" == "1" ]; then
        PSP_OPTION="--enable-pod-security-policy"
    fi

    echo "-> Creating GKE cluster..."
    gcloud beta container --project ${GCLOUD_PROJECT} clusters create ${GKE_CLUSTER_NAME} \
        --region "${GKE_CLUSTER_REGION}" --username "${GKE_ADMIN_USERNAME}" --cluster-version "${GKE_CLUSTER_VERSION}" \
        --machine-type "${GKE_MACHINE_TYPE}" --image-type "COS" --disk-type "pd-ssd" --disk-size "30" \
        --local-ssd-count "${GKE_LOCAL_SSD_COUNT}" --scopes "${GKE_GCP_SCOPES}" --num-nodes "${GKE_NODE_COUNT_PER_ZONE}" \
        --enable-cloud-logging --enable-cloud-monitoring --addons HorizontalPodAutoscaling,HttpLoadBalancing \
        --no-enable-autoupgrade --no-enable-autorepair --network "projects/${GCLOUD_PROJECT}/global/networks/default" \
        --subnetwork "projects/${GCLOUD_PROJECT}/regions/${GKE_CLUSTER_REGION}/subnetworks/default" \
        ${PSP_OPTION}

    # Export credentials for kubelet
    export_credentials

    # Create required role binding between the GCP account and the K8s cluster.
    kubectl create clusterrolebinding cluster-admin-binding --clusterrole=cluster-admin --user=$(gcloud auth list --filter=status:ACTIVE --format="value(account)")

    # set vm.max_map_count
    set_max_map_count

    # Create a default storage class that uses late binding to avoid volume zone affinity issues
    kubectl apply -f $HERE/../config/dev/gke-default-storage.yaml
    kubectl patch storageclass standard -p '{
        "metadata": {"annotations": {"storageclass.beta.kubernetes.io/is-default-class":"false"} }
    }'
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
    auth)
      auth_service_account
    ;;
    *)
      echo "Usage: gke-cluster.sh (create|delete|name|registry|credentials|auth)"; exit 1
    ;;
  esac
}

main "$@"

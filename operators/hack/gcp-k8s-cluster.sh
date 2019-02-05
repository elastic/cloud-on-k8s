#!/bin/bash

# This script takes responsibility to create a GCP GKE cluster. It has all
# of the necessary default settings so that no positional arguments have to
# be specified.

GCLOUD_PROJECT=${1:-elastic-cloud-dev}
GKE_CLUSTER_NAME=${2:-${USER//_}-dev-cluster}
GKE_CLUSTER_REGION=${3:-europe-west3}
GKE_ADMIN_USERNAME=${4:-admin}
GKE_CLUSTER_VERSION=${5:-1.11.2-gke.15}
GKE_MACHINE_TYPE=${6:-n1-highmem-4}
GKE_LOCAL_SSD_COUNT=${7:-1}
GKE_NODE_COUNT_PER_ZONE=${8:-1}
GKE_GCP_SCOPES='https://www.googleapis.com/auth/devstorage.read_only,https://www.googleapis.com/auth/logging.write,https://www.googleapis.com/auth/monitoring,https://www.googleapis.com/auth/servicecontrol,https://www.googleapis.com/auth/service.management.readonly,https://www.googleapis.com/auth/trace.append'


if gcloud beta container clusters --project "${GCLOUD_PROJECT}" describe --region "${GKE_CLUSTER_REGION}" "${GKE_CLUSTER_NAME}" > /dev/null 2>&1; then
    echo "-> GKE cluster is running."
    exit 0
fi

# Create cluster
echo "-> Creating GKE cluster..."
gcloud beta container --project ${GCLOUD_PROJECT} clusters create ${GKE_CLUSTER_NAME} \
    --region "${GKE_CLUSTER_REGION}" --username "${GKE_ADMIN_USERNAME}" --cluster-version "${GKE_CLUSTER_VERSION}" \
    --machine-type "${GKE_MACHINE_TYPE}" --image-type "COS" --disk-type "pd-ssd" --disk-size "30" \
    --local-ssd-count "${GKE_LOCAL_SSD_COUNT}" --scopes "${GKE_GCP_SCOPES}" --num-nodes "${GKE_NODE_COUNT_PER_ZONE}" \
    --enable-cloud-logging --enable-cloud-monitoring --addons HorizontalPodAutoscaling,HttpLoadBalancing \
    --no-enable-autoupgrade --no-enable-autorepair --network "projects/elastic-cloud-dev/global/networks/default" \
    --subnetwork "projects/elastic-cloud-dev/regions/europe-west3/subnetworks/default"

# Create required role binding between the GCP account and the K8s cluster.
kubectl create clusterrolebinding elastic-operators--manager-rolebinding --clusterrole=cluster-admin --user=$(gcloud auth list --filter=status:ACTIVE --format="value(account)")

#! /usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

#
# Run end-to-end tests as a K8s batch job
# Usage: ./hack/run-e2e.sh <e2e_docker_image_name> <go_tests_matcher>
#

set -eu

IMG="$1" # Docker image name
TESTS_MATCH="$2" # Expression to match go test names (can be "")

JOB_NAME="elastic-operators-e2e-tests-$(date +'%Y%m%d-%H%M%S')"
NAMESPACE="e2e"

# exit early if another job already exists
set +e
kubectl -n e2e get job $JOB_NAME && \
    echo "Job $JOB_NAME already exists, please delete it first. Exiting." && \
    exit 1
set -e

# apply e2e job
sed \
    -e "s;\$IMG;$IMG;g" \
    -e "s;\$TESTS_MATCH;$TESTS_MATCH;g" \
    -e "s;\$JOB_NAME;$JOB_NAME;g" \
    config/e2e/batch_job.yaml | \
    kubectl apply -f -

# retrieve pod responsible for running the job
pod=$(kubectl get pods -n $NAMESPACE --selector=job-name=$JOB_NAME --output=jsonpath={.items..metadata.name})
# wait until its container is started
while kubectl -n $NAMESPACE get pod $pod | grep ContainerCreating; do
    sleep 1
done
# stream logs until completion
kubectl -n $NAMESPACE logs -f $pod

# get job status (number of failures)
status=$(kubectl -n $NAMESPACE get job $JOB_NAME -o jsonpath={.status.failed})
if [[ "$status" == "" ]]; then
    echo "e2e tests success"
else
    echo "e2e tests failure"
fi

# delete job
kubectl -n e2e delete job $JOB_NAME

# exit with job status (eg. "1" if failure, "" if success)
exit $status

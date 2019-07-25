#!/usr/bin/env bash

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

JOB_NAME="eck-e2e-tests-$(LC_CTYPE=C tr -dc 'a-z0-9' < /dev/urandom | fold -w 6 | head -n 1)"
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
pod=""
retry=0
e2e_pod_creation_max_retries=30
set +e # ignore error in the retry loop
while true; do
    if [[ ${retry} -ge ${e2e_pod_creation_max_retries} ]]; then
        echo "failed to get the e2e pod name after ${e2e_pod_creation_max_retries} retries"
        exit 1
    fi
    ((retry++))
    pod=$(kubectl get pods -n ${NAMESPACE} --selector=job-name=${JOB_NAME} --output=jsonpath={.items..metadata.name})
    if [[ ! -z "${pod}" ]]; then
        break
    fi
    sleep 1;
done
set -e

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

    echo "--"
    echo "# k8s statefulset"
    echo "> kubectl get sts --all-namespaces"
    echo "--"
    kubectl get sts --all-namespaces
    echo "--"
    echo "# elastic-namespace-operator log error"
    echo "> kubectl -n elastic-namespace-operators logs elastic-namespace-operator-0  | grep error"
    echo "--"
    kubectl -n elastic-namespace-operators logs elastic-namespace-operator-0  | grep error
    echo "--"
    echo "# k8s ressources"
    echo "> kubectl -n $NAMESPACE get elastic,pods,rs,deploy,svc,cm,secrets,pvc,pv"
    echo "--"
    kubectl -n $NAMESPACE get elastic,pods,rs,deploy,svc,cm,secrets,pvc,pv
    echo "--"
    echo "# events for non-running pods"
    for pod in $(kubectl -n $NAMESPACE get pods --no-headers --field-selector=status.phase!=Running -o custom-columns=:metadata.name)
    do
        echo "--"
        echo "> kubectl -n $NAMESPACE get event --field-selector involvedObject.name=$pod"
        kubectl -n $NAMESPACE get event --field-selector involvedObject.name=$pod
        echo "--"
    done
fi

# delete job
kubectl -n e2e delete job $JOB_NAME

# exit with job status (eg. "1" if failure, "" if success)
exit $status

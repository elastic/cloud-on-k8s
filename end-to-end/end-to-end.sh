#!/usr/bin/env bash -eu

# Run a very basic end-to-end test, checking
# stack deployment "up-to a running state.
# It "will use the local kubectl with the current "k8s cluster,
# and expectes the stack-operator to be running.

# Variables that "may be overriden through environment
: "${STACK_NAME:=stack-sample}"
: "${STACK_YAML_PATH:=config/samples/deployments_v1alpha1_stack.yaml}"
: "${EXPECTED_NUM_ES_PODS:=3}"
: "${EXPECTED_NUM_KB_PODS:=1}"

# format test name
function t {
    echo
    echo "## $@"
}

# number of pods with the given grep pattern
function get_num_pods {
    local pod_pattern="$1"
    kubectl get pod | grep "$pod_pattern" | wc -l | tr -d '[:space:]'
}

# number of running pods with the given grep pattern
function get_num_running_pods {
    local pod_pattern="$1"
    kubectl get pod | grep "$pod_pattern" | grep "Running" | wc -l | tr -d '[:space:]'
}

# endpoints for the given service
function get_endpoints {
    local service_name="$1"
    kubectl get endpoints "$service_name" | tail -n +2 | awk '{print $2}' | tr -d '[:space:]'
}

# monitor tests duration
start=$(date +%s)

###
t "Checking kubectl config"
kubectl config current-context

###
t "Checking access to the kubernetes cluster"
kubectl get pod

###
t "Deleting previous stack if exists"
(kubectl delete stack $STACK_NAME &&
until
num_pods=$(get_num_pods "$STACK_NAME-es")
[[ $num_pods == "0" ]]
do
    echo "Waiting for ES pods deletion..." && sleep 5
done
) || true

###
t "Creating new stack"
kubectl apply -f $STACK_YAML_PATH

###
t "Stack should be created"
kubectl get stack $STACK_NAME | grep $STACK_NAME

###
t "ES pods should be created"
until
num_pods=$(get_num_pods "$STACK_NAME-es")
[[ $num_pods == $EXPECTED_NUM_ES_PODS ]]
do
    echo "Waiting for ES pods creation..." && sleep 5
done

###
t "Kibana pods should be created"
until
    num_pods=$(get_num_pods "$STACK_NAME-kibana")
    [[ $num_pods == $EXPECTED_NUM_KB_PODS ]]
do
    echo "Waiting for Kibana pods creation..." && sleep 5
done

###
t "Services should be created"
until
    kubectl get svc $STACK_NAME-es-discovery
    kubectl get svc $STACK_NAME-es-public
    kubectl get svc $STACK_NAME-kb
do
    echo "Waiting for services creation..." && sleep 5
done

###
t "ES pods should eventually be in a Running state"
until
    num_pods=$(get_num_running_pods "$STACK_NAME-es")
    [[ $num_pods == $EXPECTED_NUM_ES_PODS ]]
do
    echo "Waiting for ES pods to be in a running state..." && sleep 5
done

###
t "Kibana pods should eventually be in a Running state"
until
    num_pods=$(get_num_running_pods "$STACK_NAME-kibana")
    [[ $num_pods == $EXPECTED_NUM_KB_PODS ]]
do
    echo "Waiting for Kibana pods to be in a running state..." && sleep 5
done

###
t "ES public service should have endpoints"
endpoints=$(get_endpoints "$STACK_NAME-es-public")
[[ $endpoints != "" ]]

###
t "Kibana public service should have endpoints"
endpoints=$(get_endpoints "$STACK_NAME-kb")
[[ $endpoints != "" ]]

###
t "Deleting stack"
kubectl delete stack $STACK_NAME

###
t "Tests successful!"
end=$(date +%s)
echo "Duration: $((end-start))sec."

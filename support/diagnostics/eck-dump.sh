#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

# Diagnostics utility for Elastic Cloud on Kubernetes (ECK)

set -eu

help() {
  echo 'Usage: ./eck-dump.sh [OPTIONS]

Dumps Elastic Cloud on Kubernetes (ECK) info out suitable for debugging and diagnosing problems.

By default, dumps everything to stdout. You can optionally specify a directory with --output-directory.
By default only dumps things in the namespaces "elastic-system" and the current, but you can switch to
different namespaces with the --operator-namespaces and --elastic-namespaces flags.

Options:
  -N, --operator-namespaces     Namespace(s) in which operator(s) are running in (comma-separated list)
  -n, --resources-namespaces    Namespace(s) in which resources are managed      (comma-separated list)
  -o, --output-directory        Path to output dump files
  -z, --create-zip              Create an archive with the dump files (implies --output-directory)
  -v, --verbose                 Verbose mode

Dependencies:
  kubectl'
  exit
}

OPERATOR_NS=elastic-system
RESOURCES_NS="default"
OUTPUT_DIR=""
ZIP=""
VERBOSE=""

parse_args() {
  while :; do
    local flag=${1:-""} value=${2:-""}
    case $flag in
    -h|--help)
      help
    ;;
    -N|--operator-namespaces)
      OPERATOR_NS=${value:-$OPERATOR_NS}
    ;;
    -n|--resources-namespaces)
      RESOURCES_NS=${value:-${$(current_namespace):-$RESOURCES_NS}}
    ;;
    -o|--output-directory)
      OUTPUT_DIR=${value:-$OUTPUT_DIR}
      if [[ -z $OUTPUT_DIR ]]; then
        >&2 echo "flag needs an argument: --output-directory"
        exit 1
      fi
    ;;
    -z|--create-zip)
      ZIP=1
    ;;
    -v|--verbose)
      VERBOSE=1
    ;;
    esac
    shift || break
  done
}

main() {
  parse_args "$@"

  if [[ -z $OUTPUT_DIR && -n $ZIP ]]; then
    >&2 echo "flag needs to be defined : --output-directory"
    exit 1
  fi

  IFS=, # use comma as field separator for iterations
  
  # start by checking if the namespaces exist
  local all_ns="$OPERATOR_NS,$RESOURCES_NS"
  for ns in $all_ns; do
    check_namespace "$ns"
  done

  # get global info from cluster-level resources
  kubectl version   -o json | to_stdin_or_file version.json
  kubectl get nodes -o json | to_stdin_or_file nodes.json
  kubectl get podsecuritypolicies -o json | to_stdin_or_file podsecuritypolicies.json
  # describe matches by prefix
  kubectl describe clusterroles elastic | to_stdin_or_file clusterroles.txt

  # get info from the namespaces in which operators are running in 
  for ns in $OPERATOR_NS; do
    get_resources "$ns" statefulsets
    get_resources "$ns" pods
    get_resources "$ns" services
    get_resources "$ns" configmaps
    get_resources "$ns" events
    get_resources "$ns" networkpolicies
    get_resources "$ns" controllerrevisions
    get_logs "$ns"
  done

  # get info from the namespaces in which resources are managed 
  for ns in $RESOURCES_NS; do
    get_resources "$ns" statefulsets
    get_resources "$ns" replicasets
    get_resources "$ns" deployments
    get_resources "$ns" daemonsets
    get_resources "$ns" pods
    get_resources "$ns" persistentvolumes
    get_resources "$ns" persistentvolumeclaims
    get_resources "$ns" services
    get_resources "$ns" endpoints
    get_resources "$ns" configmaps
    get_resources "$ns" events
    get_resources "$ns" networkpolicies
    get_resources "$ns" controllerrevisions
    get_metadata "$ns" secrets

    # get all managed resources and their logs
    get_resources "$ns" kibana
    get_logs "$ns" common.k8s.elastic.co/type=kibana
    get_resources "$ns" elasticsearch
    get_logs "$ns" common.k8s.elastic.co/type=elasticsearch
    get_resources "$ns" apmserver
    get_logs "$ns" common.k8s.elastic.co/type=apm-server
    get_resources "$ns" enterprisesearch
    get_logs "$ns" common.k8s.elastic.co/type=enterprise-search
    get_resources "$ns" beat
    get_logs "$ns" common.k8s.elastic.co/type=beat
    get_resources "$ns" agent
    get_logs "$ns" common.k8s.elastic.co/type=agent
  done

  if [[ -n $OUTPUT_DIR ]]; then
    local dest=$OUTPUT_DIR
    if [[ -n $ZIP ]]; then
      dest=$OUTPUT_DIR-$(date +%d_%b_%Y_%H_%M_%S).tgz
      tar czf "$dest" "$OUTPUT_DIR"/*
    fi
    echo "ECK info dumped to $dest"
  fi
}

# get_resources lists resources in a specified namespace in JSON output format
get_resources() {
  local ns=$1 resources=$2
  kubectl get -n "$ns" "$resources" -o json | to_stdin_or_file "$ns"/"$resources".json
}

# get_metadata lists resources metadata in a specified namespace in JSON output format
get_metadata() {
  local ns=$1 resources=$2
  kubectl get -n "$ns" "$resources" -o=jsonpath='{range .items[*]}{.metadata}{"\n"}{end}' | to_stdin_or_file "$ns"/"$resources".json
}

# get_logs retrieves logs for all pods in a specified namespace
get_logs() {
  local ns=$1 label=${2:-""} # optional label selector
  while read -r name; do
  (
    echo "==== START logs for pod $ns/$name ===="
    kubectl -n "$ns" logs "$name" --all-containers 
    echo "==== END logs for pod $ns/$name ===="
  ) | to_stdin_or_file "$ns"/"$name"/logs.txt
  done \
  < <(list_pods_names "$ns" "$label")
}

# list_pods_names lists the names of the pods in a specified namespace
list_pods_names() {
  local ns=$1 label=${2:-""}
  if [[ -n $label ]]; then
    kubectl get pods -n "$ns" -l "$label" --no-headers=true -o name
  else
    kubectl get pods -n "$ns" --no-headers=true -o name
  fi
}

# to_stdin_or_file redirects stdin to a file if OUTPUT_DIR is defined, else to stdout
to_stdin_or_file() {
  local filepath=$1
  if [[ -n $VERBOSE ]]; then
    >&2 echo "$OUTPUT_DIR/$filepath"
  fi
  if [[ -n $OUTPUT_DIR ]]; then
    mkdir -p "$(dirname "$OUTPUT_DIR"/"$filepath")"
    cat /dev/stdin > "$OUTPUT_DIR"/"$filepath"
  else
    cat /dev/stdin
  fi
}

# check_namespace fails and exits the program if the namespace does not exist
check_namespace() {
  local ns=$1
  kubectl get namespace "$ns" >/dev/null
}

# current_namespace returns the current namespace, empty if there is none
current_namespace() {
  kubectl config view --minify --output 'jsonpath={..namespace}' 2>/dev/null
}

main "$@"

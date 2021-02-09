#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

# Script to automate the task of adding or removing the "exclude" annotation from/to all Elastic resources in a Kubernetes cluster.

set -euo pipefail

ANN_KEY=${ANN_KEY:-"eck.k8s.elastic.co/managed"}
ANN_VAL=${ANN_VAL:-"true"}
PAUSE_SECS=${PAUSE_SECS:-"900"}

remove_all() {
    mapfile -t OBJECTS < <(kubectl get elastic --all-namespaces -o=jsonpath='{range .items[*]}{.kind}{"|"}{.metadata.name}{"|"}{.metadata.namespace}{"\n"}{end}')

    for OBJ in "${OBJECTS[@]}"; do
        IFS='|' read -r -a ARGS <<< "$OBJ"

        echo "Removing $ANN_KEY annotation from ${ARGS[0]} resource ${ARGS[1]} in namespace ${ARGS[2]}"
        kubectl annotate "${ARGS[0],,}" "${ARGS[1]}" "${ANN_KEY}-" -n "${ARGS[2]}"
        echo "Waiting $PAUSE_SECS seconds"
        sleep "$PAUSE_SECS"
    done
}

add_all() {
    mapfile -t NAMESPACES < <(kubectl get ns -o=custom-columns='NAME:.metadata.name' --no-headers)

    for NS in "${NAMESPACES[@]}"; do
        echo "Adding $ANN_KEY=$ANN_VAL annotation to all Elastic resources in namespace $NS"
        kubectl annotate --overwrite elastic --all "${ANN_KEY}=${ANN_VAL}" -n "$NS"
    done
}

list_all() {
    TEMPLATE="{{ range .items }}{{ if index .metadata.annotations \"${ANN_KEY}\" }}{{ printf \"%s|%s|%s\\n\" .kind .metadata.name .metadata.namespace }}{{ end }}{{ end }}"
    mapfile -t ANNOTATED < <(kubectl get elastic --all-namespaces -o=go-template="$TEMPLATE")

    printf "NAMESPACE\tTYPE\tNAME\n"
    for OBJ in "${ANNOTATED[@]}"; do
        IFS='|' read -r -a ARGS <<< "$OBJ"
        printf   "%s\t%s\t%s\n" "${ARGS[2]}" "${ARGS[0]}" "${ARGS[1]}"
    done
}

usage() {
    echo "Usage: $0 ls|add|remove"
    echo "ls: Lists all Elastic resources that have the $ANN_KEY annotation"
    echo "add: Add the $ANN_KEY=$ANN_VAL annotation to all Elastic resources in all namespaces"
    echo "remove: Remove the $ANN_KEY annotation from all Elastic resources in all namespaces"
    exit 2
}


if [[ "$#" -ne 1 ]]; then
    usage
fi

case "$1" in
    ls)
        list_all
        ;;
    add)
        add_all
        ;;
    remove)
        remove_all
        ;;
    help)
        usage
        ;;
    *)
        echo "Unknown action '$1'"
        usage
        ;;
esac

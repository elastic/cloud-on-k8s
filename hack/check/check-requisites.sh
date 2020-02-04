#! /usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

# This script will check for any missing tool required to contribute to this project.

set -eu

all_found=true

function check {
    local exec_name="$@"
    printf "Checking for $exec_name... "
    if ! command -v $exec_name >/dev/null 2>&1; then
        printf "missing!"
        all_found=false
    else
        printf "found."
    fi
    printf "\n"
}

function check_oneof {
    local found_one=false

    for exec_name in $@
    do
        printf "Checking for (optional) $exec_name... "
        if ! command -v $exec_name >/dev/null 2>&1; then
            printf "not found."
        else
            printf "found."
            found_one=true
        fi
        printf "\n"
    done

    if [[ "$found_one" != "true" ]]; then
        echo "At least one of [$@] must be installed."
        all_found=false
    fi
}

check go
check golangci-lint
check kubectl
check kubebuilder
check_oneof gcloud minikube kind

echo
if [[ "$all_found" != "true" ]]; then
    echo "Some tools are missing."
    exit 1
else
    echo "All tools are present."
fi

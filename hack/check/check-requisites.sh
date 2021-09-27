#! /usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# This script will check for any missing tool required to contribute to this project.

set -eu

MIN_GO_VERSION=16
MIN_KUBECTL_VERSION=14
MIN_DOCKER_VERSION=19

green="\e[32m"
red="\e[31m"
reset="\e[39m"
all_found=true

check() {
    local exec_name=$*
    printf "Checking for %s..." "$exec_name"
    if ! command -v "$exec_name" >/dev/null 2>&1; then
        printf "%bnot found%b" "${red}" "${reset}"
        all_found=false
    else
        printf "%bfound%b" "${green}" "${reset}"
    fi
    printf "\n"
}

check_oneof() {
    local found_one=false

    for exec_name in "$@"
    do
        printf "Checking for (optional) %s..." "$exec_name"
        if ! command -v "$exec_name" >/dev/null 2>&1; then
            printf "%bnot found%b" "${red}" "${reset}"
        else
            printf "%bfound%b" "${green}" "${reset}"
            found_one=true
        fi
        printf "\n"
    done

    if [[ "$found_one" != "true" ]]; then
        echo "At least one of [$*] must be installed."
        all_found=false
    fi
}

check_go_version() {
    local major
    major=$(go version | sed -E "s|.* go([1-9]).[0-9]*[0-9.]* .*|\1|")
    local minor
    minor=$(go version | sed -E "s|.* go[1-9].([0-9]*)[0-9.]* .*|\1|")

    printf "Checking for go >= 1.%s..." "$MIN_GO_VERSION"
    if [[ "$major" -gt 1 ]] || [[ "$minor" -ge $MIN_GO_VERSION ]]; then
        printf "%bok%b (%s.%s)" "${green}" "${reset}" "$major" "$minor"
    else
        printf "%bko$%b (%s.%s)" "${red}" "${reset}" "$major" "$minor"
        all_found=false
    fi
    printf "\n"
}

check_kubectl_version() {
    local major
    major=$(kubectl --client=true version | grep -Eo 'Major:"[0-9]*' | grep -Eo '[0-9]+')
    local minor
    minor=$(kubectl --client=true version | grep -Eo 'Minor:"[0-9]*' | grep -Eo '[0-9]+')

    printf "Checking for kubectl >= 1.%s... " "$MIN_KUBECTL_VERSION"
    if [[ "$major" -gt 1 ]] || [[ "$minor" -ge $MIN_KUBECTL_VERSION ]]; then
        printf "%bok%b (%s.%s)" "${green}" "${reset}" "$major" "$minor"
    else
        printf "%bko$%b (%s.%s)" "${red}" "${reset}" "$major" "$minor"
        all_found=false
    fi
    printf "\n"
}

check_docker_version() {
    local major 
    major=$(docker version -f '{{.Client.Version}}' | sed -E 's|([0-9]+)\.[0-9]+\.[0-9]+.*|\1|')
    local docker_version
    docker_version=$(docker version -f '{{.Client.Version}}')

    printf "Checking for Docker >= %s.0.0... " "$MIN_DOCKER_VERSION"
    if [[ "$major" -gt $MIN_DOCKER_VERSION ]]; then
        printf "%bok%b (%s)" "${green}" "${reset}" "$docker_version"
    else
        printf "%bko$%b (%s)" "${red}" "${reset}" "$docker_version" 
        all_found=false
    fi
    printf "\n"
}


check go
check golangci-lint
check kubectl
check kubebuilder
check_oneof gcloud minikube kind
check_go_version
check_kubectl_version
check_docker_version

echo
if [[ "$all_found" != "true" ]]; then
    printf "%bError%b: some requirements not satified.\n" "${red}" "${reset}" >&2
    exit 1
else
    printf "%bOK%b: all requirements met.\n" "${green}" "${reset}"
fi

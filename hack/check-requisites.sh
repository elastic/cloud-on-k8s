#! /usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

# This script will check for any missing tool required to contribute to this project.

set -eu

MIN_GO_VERSION=13
MIN_KUBECTL_VERSION=14

green="\e[32m"
red="\e[31m"
reset="\e[39m"
all_found=true

check() {
    local exec_name="$@"
    printf "Checking for $exec_name... "
    if ! command -v $exec_name >/dev/null 2>&1; then
        printf "${red}missing${reset}"
        all_found=false
    else
        printf "${green}found${reset}"
    fi
    printf "\n"
}

check_oneof() {
    local found_one=false

    for exec_name in $@
    do
        printf "Checking for (optional) $exec_name... "
        if ! command -v $exec_name >/dev/null 2>&1; then
            printf "${red}missing${reset}"
        else
            printf "${green}found${reset}"
            found_one=true
        fi
        printf "\n"
    done

    if [[ "$found_one" != "true" ]]; then
        echo "At least one of [$@] must be installed."
        all_found=false
    fi
}

check_go_version() {
    local major=$(go version | sed -r "s|.* go([1-9]).[0-9]*[0-9.]* .*|\1|")
    local minor=$(go version | sed -r "s|.* go[1-9].([0-9]*)[0-9.]* .*|\1|")

    printf "Checking for go >= 1.$MIN_GO_VERSION... "
    if [[ "$major" -gt 1 ]] || [[ "$minor" -ge $MIN_GO_VERSION ]]; then
        printf "${green}ok${reset} ($major.$minor)"
    else
        printf "${red}ko${reset} ($major.$minor)"
        all_found=false
    fi
    printf "\n"
}

check_kubectl_version() {
    local major=$(kubectl --client=true version | grep -Eo 'Major:"[0-9]*' | grep -o '[0-9]*')
    local minor=$(kubectl --client=true version | grep -Eo 'Minor:"[0-9]*' | grep -o '[0-9]*')

    printf "Checking for kubectl >= 1.$MIN_KUBECTL_VERSION... "
    if [[ "$major" -gt 1 ]] || [[ "$minor" -ge $MIN_KUBECTL_VERSION ]]; then
        printf "${green}ok${reset} ($major.$minor)"
    else
        printf "${red}ko${reset} ($major.$minor)"
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

echo
if [[ "$all_found" != "true" ]]; then
    printf "${red}Error${reset}: requirements could not be met.\n" >&2
    exit 1
else
    printf "${green}OK${reset}: all requirements met.\n"
fi

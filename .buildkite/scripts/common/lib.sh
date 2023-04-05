#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# retry function
# -------------------------------------
# Retry a command for a specified number of times until the command exits successfully.
# Retry wait period backs off exponentially after each retry.
#
# The first argument should be the number of retries. 
# Remainder is treated as the command to execute.
# -------------------------------------
retry() {
  local retries=$1
  shift

  local count=0
  until "$@"; do
    exit=$?
    wait=$((2 ** count))
    count=$((count + 1))
    if [ $count -lt "$retries" ]; then
      printf "Retry %s/%s exited %s, retrying in %s seconds...\n" "$count" "$retries" "$exit" "$wait" >&2
      sleep $wait
    else
      printf "Retry %s/%s exited %s, no more retries left.\n" "$count" "$retries" "$exit" >&2
      return $exit
    fi
  done
  return 0
}

project_path() {
    # in "k8s" agent container
    if [[ "${BUILDKITE_AGENT_NAME:-}" =~ k8s ]]; then
        # current path of the k8s agent
        pwd
    # in "docker" agent container run in a "gcp" agent vm
    else
        # docker agent image workdir
        echo /go/src/github.com/elastic/cloud-on-k8s
    fi
}

is_not_buildkite() {
    [[ "${BUILDKITE_BUILD_NUMBER:-}" == "" ]] && return 0 || return 1
}

is_tag() {
    [[ "${BUILDKITE_TAG:-}" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+ ]] && return 0 || return 1
}

is_merge_main() {
    [[ "${BUILDKITE_BRANCH:-}" == "main" && "${BUILDKITE_SOURCE:-}" != "schedule" ]] && return 0 || return 1
}

is_nightly_main() {
    [[ "${BUILDKITE_BRANCH:-}" == "main" && "${BUILDKITE_SOURCE:-}" == "schedule" ]] && return 0 || return 1
}

is_pr() {
    [[ "${BUILDKITE_PULL_REQUEST:-}" != "" && "${BUILDKITE_PULL_REQUEST:-}" != "false" ]] || \
    [[ "${GITHUB_PR_TRIGGER_COMMENT:-}" != ""  ]] && return 0 || return 1
}

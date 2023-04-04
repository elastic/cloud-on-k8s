#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

set -eu

WD="$(cd "$(dirname "$0")"; pwd)"
ROOT="$WD/../../.."

current_version=$(cat VERSION)
current_sha1=$(git rev-parse --short=8 --verify HEAD)

setenv=$ROOT/.buildkite/scripts/build/setenv.sh 

TOTAL=0
OK=0
KO=0
assert_image() {
    TOTAL=$((TOTAL+1))
    expected=$1
    got=$(make print-operator-image)
    if ! grep -q "$expected$" <<< "$got"; then
        KO=$((KO+1))
        echo " 🔴 expected: $expected"
        echo " 🔴 got:      $got"
    else
        OK=$((OK+1))
        echo " 🟢  $got"
    fi
    echo "--"
}

export BUILDKITE_BUILD_NUMBER=999999999999999

echo "test trigger_pr"; BUILDKITE_PULL_REQUEST="4242" $setenv > /dev/null
assert_image "docker.elastic.co/eck-ci/eck-operator-pr:4242-${current_sha1}"

echo "test trigger_nightly_main"; BUILDKITE_BRANCH="main" BUILDKITE_SOURCE="schedule" $setenv > /dev/null
assert_image "docker.elastic.co/eck-snapshots/eck-operator:${current_version}-${current_sha1}"

echo "test trigger_nightly_main-dev"; BUILDKITE_BRANCH="main" BUILDKITE_SOURCE="schedule" OPERATOR_VERSION_SUFFIX=dev $setenv > /dev/null
assert_image "docker.elastic.co/eck-snapshots/eck-operator:${current_version}-${current_sha1}-dev"

echo "test trigger_merge_main"; BUILDKITE_BRANCH="main" $setenv > /dev/null
assert_image "docker.elastic.co/eck-snapshots/eck-operator:${current_version}-${current_sha1}"

echo "test trigger_tag"; BUILDKITE_TAG="v1.2.3" $setenv build > /dev/null
assert_image "docker.elastic.co/eck/eck-operator:1.2.3"

echo "test trigger_vtag"; BUILDKITE_TAG="v1.2.3" $setenv build > /dev/null
assert_image "docker.elastic.co/eck/eck-operator:1.2.3"

echo "test trigger_vtagbc"; BUILDKITE_TAG="v1.2.3-bc1" $setenv build > /dev/null
assert_image "docker.elastic.co/eck/eck-operator:1.2.3-bc1"

echo "test trigger_branch"; BUILDKITE_BRANCH="4.2" BUILDKITE_PULL_REQUEST="false" $setenv > /dev/null
assert_image "docker.elastic.co/eck-ci/eck-operator-branch:${current_version}-${current_sha1}"

echo "Passed: $OK/$TOTAL"
if [[ "$KO" -gt 0 ]]; then
    exit 1
fi

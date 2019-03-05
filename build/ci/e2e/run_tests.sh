#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

# Script for running e2e tests and deleting GKE cluster in case of test fail

make -C operators ci-e2e
ec=$?
if [ $ec -ne 0 ]; then
    echo "-> Deleting GKE cluster ..."
    make -C operators delete-gke
fi
exit $ec

#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

case "$1" in
    driver) ./driver.sh;;
    provisioner) ./provisioner;;
    *) echo "Usage: entrypoint.sh <driver|provisioner>";;
esac

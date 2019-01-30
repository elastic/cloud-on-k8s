#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

set -eu

# driver client in the container
IN_CONTAINER_PATH="/app/driverclient"
# mounted volume to copy the driver client into
MOUNT_DIR="/flexbin/volumes.k8s.elastic.co~elastic-local"
# name of the storage class that should map the binary file name
STORAGE_CLASS="elastic-local"

echo "Copying $IN_CONTAINER_PATH to $MOUNT_DIR/$STORAGE_CLASS..."
mkdir -p $MOUNT_DIR
cp "$IN_CONTAINER_PATH" "$MOUNT_DIR/$STORAGE_CLASS"
echo "Success."

echo "Starting $STORAGE_CLASS driver daemon..."
./driverdaemon

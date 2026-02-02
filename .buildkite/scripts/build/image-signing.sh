#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to generate the list of container image digests for signing.
# Reads the list of images from buildkite metadata (set by gen-drivah.toml.sh)
# and outputs the digests to eck-container-images-digest.txt.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")"; pwd)/../../.."

retry() { "$ROOT/hack/retry.sh" 5 "$@"; }

# Get image digest from registry
get_image_digest() {
    local image_ref="$1"
    local digest

    # Attempt to get the digest from the multi-arch manifest using docker manifest inspect which
    # works with remote images without needing to pull the image.
    if ! command -v docker >/dev/null 2>&1; then
        echo "Error: docker command not found" >&2
        return 1
    fi

    # Get raw manifest and compute its digest. We must take this approach as the docker build is a multi-arch build which contains manifests for both the amd64 and arm64 architectures.
    local manifest
    local stderr_file
    stderr_file=$(mktemp)
    if ! manifest=$(retry docker buildx imagetools inspect "$image_ref" --raw 2>"$stderr_file"); then
        echo "Error: failed to inspect $image_ref" >&2
        cat "$stderr_file" >&2
        rm -f "$stderr_file"
        return 1
    fi
    rm -f "$stderr_file"

    if [[ -z "$manifest" || "$manifest" == "null" ]]; then
        echo "Error: could not extract manifest from $image_ref" >&2
        return 1
    fi

    echo "sha256:$(echo -n "$manifest" | sha256sum | cut -d' ' -f1)"
}

main() {
    local output_file="eck-container-images-digest.txt"

    # Initialize output file
    true > "$output_file"

    # Get list of images to sign from buildkite metadata
    local images_list
    if ! images_list=$(buildkite-agent meta-data get images-to-sign 2>&1); then
        echo "Error: Failed to get images-to-sign metadata. Was gen-drivah.toml.sh run?" >&2
        echo "$images_list" >&2
        exit 1
    fi

    if [[ -z "$images_list" ]]; then
        echo "Error: images-to-sign buildkite metadata is empty" >&2
        exit 1
    fi

    echo "Processing images for signing:"
    echo "$images_list"
    echo

    # Process each image
    while IFS= read -r image_ref; do
        [[ -z "$image_ref" ]] && continue

        echo -n "Getting digest for $image_ref ... "
        local digest
        if digest=$(get_image_digest "$image_ref"); then
            echo "${image_ref}@${digest}" >> "$output_file"
            echo "OK"
        else
            echo "FAILED"
            echo "Warning: Could not get digest for $image_ref" >&2
        fi
    done <<< "$images_list"

    echo
    echo "Generated image digests file: $output_file"
    if [[ -s "$output_file" ]]; then
        cat "$output_file"
    else
        echo "Warning: No image digests found." >&2
        exit 1
    fi
}

main

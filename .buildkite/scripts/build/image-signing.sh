#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to generate the list of container image digests for signing.
# Reads the list of images from buildkite metadata (set by gen-drivah.toml.sh)
# and outputs the digests to /tmp/eck-container-images-digest.txt.

set -euo pipefail

# Get image digest from registry
get_image_digest() {
    local image_ref="$1"
    local digest

    # Attempt to get the digest from the multi-arch manifest using docker manifest inspect (works with remote images)
    if ! command -v docker >/dev/null 2>&1; then
        echo "Error: docker command not found" >&2
        return 1
    fi

    local manifest_output
    if ! manifest_output=$(docker manifest inspect "$image_ref" 2>&1); then
        echo "Error: docker manifest inspect failed for $image_ref" >&2
        echo "$manifest_output" >&2
        return 1
    fi

    # Extract digest from manifest (look for "digest" field)
    digest=$(echo "$manifest_output" | grep -oE '"digest"[[:space:]]*:[[:space:]]*"sha256:[a-f0-9]+"' | \
        head -1 | grep -oE 'sha256:[a-f0-9]+' || echo "")
    if [[ -n "$digest" ]]; then
        echo "$digest"
        return 0
    fi

    echo "Error: could not extract digest from manifest for $image_ref" >&2

    return 1
}

main() {
    local output_file="/tmp/eck-container-images-digest.txt"

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
        echo "Error: images-to-sign builtekite metadata is empty" >&2
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

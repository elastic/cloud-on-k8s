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
    local crane_output

    # Get the multi-arch manifest list digest directly with crane.
    if ! command -v crane >/dev/null 2>&1; then
        echo "Error: crane command not found" >&2
        return 1
    fi

    if ! crane_output=$(retry crane digest "$image_ref" 2>&1); then
        echo "Error: failed to get digest for $image_ref" >&2
        if [[ -n "$crane_output" ]]; then
            echo "$crane_output" >&2
        fi
        return 1
    fi
    digest="$crane_output"

    if [[ -z "$digest" || "$digest" == "null" ]]; then
        echo "Error: could not extract digest from $image_ref" >&2
        if [[ -n "$crane_output" ]]; then
            echo "crane output: $crane_output" >&2
        fi
        return 1
    fi

    echo "$digest"
}

main() {
    local output_file="eck-container-images-digest.txt"
    local metadata_output

    # Initialize output file
    true > "$output_file"

    # Get list of images to sign from buildkite metadata
    local images_list
    if ! metadata_output=$(buildkite-agent meta-data get images-to-sign 2>&1); then
        echo "Error: Failed to get images-to-sign metadata. Was gen-drivah.toml.sh run?" >&2
        if [[ -n "$metadata_output" ]]; then
            echo "$metadata_output" >&2
        fi
        exit 1
    fi
    images_list="$metadata_output"

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

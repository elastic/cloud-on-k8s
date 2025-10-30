#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2
# you may not use this file except in compliance with the Elastic License 2.0.

# This script compares the Go module dependencies between two Git branches
# by comparing the contents of their respective go.mod files.

set -euo pipefail

# Check for the correct number of arguments
if [ "$#" -ne 2 ]; then
    echo "Usage: $0 <branch1> <branch2>"
    exit 1
fi

BRANCH1=$1
BRANCH2=$2

# Function to get go.mod dependencies from a git branch
get_deps() {
    local branch=$1
    # The `|| true` at the end is to prevent the script from exiting if git show fails
    # in case a branch doesn't exist. We handle that error explicitly.
    git show "$branch:go.mod" 2>/dev/null | grep '^\s\+' | grep -v '// indirect' | sort || {
        echo "Error: Failed to get dependencies from branch '$branch'. Does the branch exist and have a go.mod file?" >&2
        exit 1
    }
}

# Get dependencies for both branches
DEPS1=$(get_deps "$BRANCH1")
DEPS2=$(get_deps "$BRANCH2")

# Use associative arrays to store dependencies
declare -A deps1
declare -A deps2

# Populate deps1
while read -r dep ver; do
    if [[ -n "$dep" ]]; then
        deps1["$dep"]="$ver"
    fi
done <<< "$DEPS1"

# Populate deps2
while read -r dep ver; do
    if [[ -n "$dep" ]]; then
        deps2["$dep"]="$ver"
    fi
done <<< "$DEPS2"

# Collect changes in a variable for sorting
changes=""

# Find updated and added dependencies
for dep in "${!deps2[@]}"; do
    ver2="${deps2[$dep]}"
    if [[ -v "deps1[$dep]" ]]; then
        ver1="${deps1[$dep]}"
        if [[ "$ver1" != "$ver2" ]]; then
            changes+="$dep $ver1 => $ver2\n"
        fi
        # remove from deps1 to track handled dependencies
        unset "deps1[$dep]"
    else
        changes+="$dep => $ver2\n"
    fi
done

# Find removed dependencies
for dep in "${!deps1[@]}"; do
    ver1="${deps1[$dep]}"
    changes+="$dep $ver1 => REMOVED\n"
done

# Sort and print the changes
echo -e "$changes" | sort

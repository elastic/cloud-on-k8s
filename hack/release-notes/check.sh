#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script that helps identifying changes that have been labeled as belonging to certain release version
# where the corresponding commits are not in the release branch.
# Its main purpose is to discover missing backports.
#
# Usage: check.sh TAG

set -uo pipefail

WD="$(cd "$(dirname "$0")" || return; pwd)"
ROOT="$WD/../.."

get-prev-version() {
    local version=$1
    git tag | grep -v '-' | grep -B1 "$version" | head -1
}

list-merged-commits() {
    local nextVersion=$1
    local prevVersion=$2
    git log --pretty=oneline "$prevVersion..$nextVersion" | grep -o "#[0-9]*" | sort
}

list-release-notes-pr-commits() {
    local version=$1
    version=${version#v} # strip v prefix
    grep -o 'pull}[0-9]*' "$ROOT/docs/release-notes/$version.asciidoc" | sed 's/pull}/#/' | sort
}

count-release-notes-pr-without-merged-commit() {
    local nextVersion=$1
    local prevVersion=$2
    diff \
        <(list-merged-commits "$nextVersion" "$prevVersion") \
        <(list-release-notes-pr-commits "$nextVersion") \
        | grep '>'
}

main() {
    local nextVersion=$1
    local prevVersion=${2:-$(get-prev-version "$nextVersion")}

    echo "Compare 'merged PR from $prevVersion to $nextVersion' with 'release-notes/$nextVersion'"
    
    prs=$(count-release-notes-pr-without-merged-commit "$nextVersion" "$prevVersion")
    if [[ -z "$prs" ]]; then
        echo "✅ LGTM"
    else
        echo "❌ Error: no commit found for the following issues:"
        count-release-notes-pr-without-merged-commit "$nextVersion" "$prevVersion"
    fi
}

main "$@"

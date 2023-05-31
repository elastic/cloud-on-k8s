#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

usage() {
	echo '
Usage: update-stack-version.sh PREVIOUS_VERSION NEW_VERSION

Update the Elastic Stack version.
Note: Make sure to escape dots in the previous version.

Example:
	update-stack-version.sh 7\.11\.2 7.12.0'
}

# Elastic Stack versions
PREVIOUS_VERSION="$1"
NEW_VERSION="$2"

[[ -z "$PREVIOUS_VERSION" ]] && usage && exit
[[ -z "$NEW_VERSION" ]] && usage && exit

set -eu

# Use the "-i.bak" trick to be compatible with both Linux and OSX
bump_version() {
	sed -i.bak -E "s|${PREVIOUS_VERSION}|${NEW_VERSION}|g" "$1"
	rm "$1.bak"
}

for_all_yaml_do() {
	local function="$1"
	# Directories containing Yaml files with version references to replace
	# Note: hack/operatorhub/config.yaml will need to be updated manually
	local dirs=(config/samples config/recipes config/e2e test/e2e deploy/eck-stack)
	LC_CTYPE=C LANG=C find "${dirs[@]}" -type f -iname \*.yaml \
		| while read -r file; do "$function" "$file"; done
}

for_all_yaml_do bump_version
bump_version test/e2e/test/version.go
bump_version test/e2e/stack_test.go
bump_version hack/operatorhub/config.yaml
bump_version Makefile

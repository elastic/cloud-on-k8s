#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

#
# Update the Elastic Stack version
# Usage: ./hack/update-stack-version.sh <previous_version> <new_version>
#

set -eu

# Elastic Stack versions
PREVIOUS_VERSION="$1"
NEW_VERSION="$2"

# Use the "-i.bak" trick to be compatible with both Linux and OSX
bump_version() {
	sed -i.bak -E "s|${PREVIOUS_VERSION}|${NEW_VERSION}|g" "$1"
	rm "$1.bak"
}

for_all_yaml_do() {
	local function="$1"
	# Directories containing Yaml files with version references to replace
	# Note: hack/operatorhub/config.yaml will need to be updated manually
	local dirs=(config/samples config/recipes config/e2e test/e2e)
	LC_CTYPE=C LANG=C find "${dirs[@]}" -type f -iname \*.yaml \
		| while read -r file; do "$function" "$file"; done
}

for_all_yaml_do bump_version
bump_version test/e2e/test/version.go
bump_version Makefile

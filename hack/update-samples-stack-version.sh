#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

#
# Update the Elastic stack version in samples 
# Usage: ./hack/update-samples-stack-version.sh <version>
#

set -eu

# Elastic stack version
VERSION="$1"

# Directories containing version references to replace
# Note: hack/operatorhub/config.yaml will need to be updated manually
dirs=(config/samples config/recipes config/e2e test/e2e)

# For all yaml files in the directory trees, replace the existing version with sed.
# We use the "-i.bak" trick to be compatible with both Linux and OSX.
# We are replacing occurrences of:
# - version: 1.2.3<EOL>
# - version: "1.2.3"<EOL>
LC_CTYPE=C LANG=C find "${dirs[@]}" -type f -iname \*.yaml \
    -exec sed -i.bak -E "s|version: \"?[0-9]+\.[0-9]+\.[0-9]+\"?$|version: $VERSION|g" "{}" \; \
    -exec rm "{}.bak" \;

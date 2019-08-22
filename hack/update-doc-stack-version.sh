#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

#
# Update the Elastic stack version in samples and documentation.
# Usage: ./hack/update-doc-stack-version.sh <version>
#

set -eu

# Elastic stack version
VERSION="$1"

# Directories containing version references to replace
: "${DIRS:="config/samples ../docs"}"

# For all yaml and asciidoc files in the directory trees, replace the existing version with sed.
# We use the "-i.bak" trick to be compatible with both Linux and OSX.
# We are replacing occurrences of:
# - version: 1.2.3<EOL>
# - version: "1.2.3"<EOL>
# - quickstart    green     1         1.2.3    (special case for es & apm quickstart)
LC_CTYPE=C LANG=C find ${DIRS} -type f \( -iname \*.asciidoc -o -iname \*.yaml \) \
    -exec sed -i.bak -E "s|version: \"?[0-9]\.[0-9]\.[0-9]\"?$|version: $VERSION|g" "{}" \; \
    -exec sed -i.bak -E "s|quickstart[[:space:]]+green[[:space:]]+1[[:space:]]+[0-9]\.[0-9]\.[0-9]  |quickstart    green     1         $VERSION  |g" "{}" \; \
    -exec rm "{}.bak" \;

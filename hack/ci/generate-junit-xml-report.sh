#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to generate a JUnit XML report from a JSON go test output.
#

set -euo pipefail

main() {
    local input_file=${1:-"input file"}
    local xml_report=${input_file%.*}.xml

    # exit without error when there is no input file
    if [[ ! -f $input_file ]]; then
        echo "No $input_file to generate a JUnit XML report."
        exit 0
    fi
  
    # temporary filter out lines containing a space in the timestamp,
    # see https://github.com/elastic/cloud-on-k8s/issues/3560.
    gotestsum \
        --junitfile "$xml_report" \
        --raw-command grep -v '"Time":"[^"]*\s[^"]*"' "$input_file" || \
    ( \
        echo "Failed to generate a JUnit XML report."
        # print the input file for further debugging
        echo " --- $input_file - START ---"
        cat "$input_file"
        echo " --- $input_file - END   ---"
        exit 1
    )
}

main "$@"

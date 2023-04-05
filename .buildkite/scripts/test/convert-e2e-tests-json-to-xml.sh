#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to convert JSON go tests result to JUnit XML tests report.

set -u

main() {
    local json_report=${1:-"json report"}
    local xml_report=${json_report%.*}.xml

    # fail if no json report file
    if [[ ! -f "$json_report" ]]; then
        echo "Error: $json_report file not found to generate JUnit XML report"
        exit 1
    fi

    # temporary filter out lines containing a space in the timestamp,
    # see https://github.com/elastic/cloud-on-k8s/issues/3560.
    gotestsum \
        --junitfile "$xml_report" \
        --raw-command grep -v '"Time":"[^"]*\s[^"]*"' "$json_report" >/dev/null || \
    ( \
        echo "Failed to generate a JUnit XML report."
        # print the input file for further debugging
        echo " --- $json_report - START ---"
        cat "$json_report"
        echo " --- $json_report - END   ---"
        exit 1
    )
}

main "$@"

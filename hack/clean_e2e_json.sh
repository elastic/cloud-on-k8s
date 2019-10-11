#! /usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

while read -r line
do
  [[ $line == *"{\"Time\":"* ]] && echo "$line"
done <e2e-tests.json > temp
mv temp e2e-tests.json

#!/usr/bin/env bash

# ELASTICSEARCH CONFIDENTIAL
# __________________
#
#  Copyright Elasticsearch B.V. All rights reserved.
#
# NOTICE:  All information contained herein is, and remains
# the property of Elasticsearch B.V. and its suppliers, if any.
# The intellectual and technical concepts contained herein
# are proprietary to Elasticsearch B.V. and its suppliers and
# may be covered by U.S. and Foreign Patents, patents in
# process, and are protected by trade secret or copyright
# law.  Dissemination of this information or reproduction of
# this material is strictly forbidden unless prior written
# permission is obtained from Elasticsearch B.V.

set -u

mapfile -t modules < <(
  find . -type f \
    -and \( -not -path './.git/*' \) \
    -and \( -name go.mod \)
)

status=0

for modfile in "${modules[@]}"; do
  dir=$(dirname "${modfile}")
  (
    set -e
    cd "${dir}"
    echo ""
    echo -e "running \e[0;35m${*}\e[0m"
    echo -e "for     \e[0;35m${dir}\e[0m"
    env "${@}"
  )
  rc=$?
  if [[ ${rc} -ne 0 ]]; then
    status=${rc}
  fi
done

exit ${status}

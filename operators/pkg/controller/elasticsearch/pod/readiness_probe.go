// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package pod

// DefaultReadinessProbeScript is the verbatim shell script that acts as a readiness probe
const DefaultReadinessProbeScript string = `
#!/usr/bin/env bash
# Consider a node to be healthy if it has a master registered
CURL_TIMEOUT=3

http_status_code () {
local url="$1"
if [ -n "${PROBE_USERNAME}" ] && [ -f "${PROBE_PASSWORD_FILE}" ]; then
  PROBE_PASSWORD=$(<$PROBE_PASSWORD_FILE)
  BASIC_AUTH="-u ${PROBE_USERNAME}:${PROBE_PASSWORD}"
else
  BASIC_AUTH=''
fi
curl -o /dev/null -w "%{http_code}" --max-time $CURL_TIMEOUT -XGET -s -k ${BASIC_AUTH} ${url}
}

check_master () {
  local endpoint="$1"
  status=$(http_status_code "$endpoint/_cat/master")
  echo "status $status"
  if [[ $status == "200" ]]; then
    return 0
  else
    return 1
  fi
}

# try https first, or fallback to http
(check_master https://localhost:9200) || (check_master http://localhost:9200)

`

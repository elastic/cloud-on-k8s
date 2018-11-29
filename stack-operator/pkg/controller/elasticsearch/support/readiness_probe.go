package support

// DefaultReadinessProbeScript is the verbatim shell script that acts as a readiness probe
const DefaultReadinessProbeScript string = `
#!/usr/bin/env bash
# Consider a node to be healthy if it has a master registered
CURL_TIMEOUT=3

http_status_code () {
local path="${1}"
if [ -n "${PROBE_USERNAME}" ] && [ -f "${PROBE_PASSWORD_FILE}" ]; then
  PROBE_PASSWORD=$(<$PROBE_PASSWORD_FILE)
  BASIC_AUTH="-u ${PROBE_USERNAME}:${PROBE_PASSWORD}"
else
  BASIC_AUTH=''
fi
curl -o /dev/null -w "%{http_code}" --max-time $CURL_TIMEOUT -XGET -s -k ${BASIC_AUTH} ${READINESS_PROBE_PROTOCOL:-http}://127.0.0.1:9200${path}
}

status=$(http_status_code "/_cat/master")
echo "status $status"
if [[ $status == "200" ]]; then
	exit 0
else
	exit 1
fi
`

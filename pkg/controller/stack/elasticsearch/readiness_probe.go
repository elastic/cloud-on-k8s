package elasticsearch

// defaultReadinessProbeScript is the verbatim shell script that acts as a readiness probe
const defaultReadinessProbeScript string = `
#!/usr/bin/env bash -e
# If the node is starting up wait for the cluster to be green
# Once it has started only check that the node itself is responding
START_FILE=/tmp/.es_start_file
PROBE_PASSWORD=$(cat /$PROBE_SECRET_MOUNT/$PROBE_USERNAME)

http () {
local path="${1}"
if [ -n "${PROBE_USERNAME}" ] && [ -n "${PROBE_PASSWORD}" ]; then
  BASIC_AUTH="-u ${PROBE_USERNAME}:${PROBE_PASSWORD}"
else
  BASIC_AUTH=''
fi
curl -XGET -s -k --fail ${BASIC_AUTH} http://127.0.0.1:9200${path}
}

if [ -f "${START_FILE}" ]; then
	echo 'Elasticsearch is already running, lets check the node is healthy'
	http "/"
	else
	echo 'Waiting for elasticsearch cluster to become green'
	if http "/_cluster/health?wait_for_status=green&timeout=1s" ; then
		touch ${START_FILE}
		exit 0
	else
		echo 'Cluster is not yet green'
		exit 1
	fi
fi
`

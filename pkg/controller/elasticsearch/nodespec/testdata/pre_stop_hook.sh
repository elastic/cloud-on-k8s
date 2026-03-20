#!/usr/bin/env bash

set -uo pipefail

# This script will wait for up to $PRE_STOP_ADDITIONAL_WAIT_SECONDS before allowing termination of the Pod
# This slows down the process shutdown and allows to make changes to the pool gracefully, without blackholing traffic when DNS
# still contains the IP that is already inactive.
# As this runs in parallel to grace period after which process is SIGKILLed,
# it should be set to allow enough time for the process to gracefully terminate.
# It allows kube-proxy to refresh its rules and remove the terminating Pod IP.
# Kube-proxy refresh period defaults to every 30 seconds, but the operation itself can take much longer if
# using iptables with a lot of services, in which case the default 30sec might not be enough.
# Also gives some additional bonus time to in-flight requests to terminate, and new requests to still
# target the Pod IP before Elasticsearch stops.
PRE_STOP_ADDITIONAL_WAIT_SECONDS=${PRE_STOP_ADDITIONAL_WAIT_SECONDS:=50}

# PRE_STOP_SHUTDOWN_TYPE controls the type of shutdown that will be communicated to Elasticsearch. This should not be
# changed to anything but restart. Specifically setting remove can lead to extensive data migration that might exceed the
# terminationGracePeriodSeconds and lead to an incomplete shutdown.
shutdown_type=${PRE_STOP_SHUTDOWN_TYPE:=restart}

# capture response bodies in a temp file for better error messages and to extract necessary information for subsequent requests
resp_body=$(mktemp)
# shellcheck disable=SC2064
trap "rm -f $resp_body" EXIT

script_start=$(date +%s)

# compute time in seconds since the given start time
function duration() {
  local start=$1
  end=$(date +%s)
  echo $((end-start))
}

# use DNS errors as a proxy to abort this script early if there is no chance of successful completion
# DNS errors are for example expected when the whole cluster including its service is being deleted
# and the service URL can no longer be resolved even though we still have running Pods.
max_dns_errors=${PRE_STOP_MAX_DNS_ERRORS:=2}
global_dns_error_cnt=0

function request() {
  local status exit
  status=$(curl -k -sS -o "${resp_body}" -w "%{http_code}" "$@")
  exit=$?
  if [ "$exit" -ne 0 ] || [ "$status" -lt 200 ] || [ "$status" -gt 299 ]; then
    # track curl DNS errors separately
    if [ "$exit" -eq 6 ]; then ((global_dns_error_cnt++)); fi
    # make sure we have a non-zero exit code in the presence of errors
    if [ "$exit" -eq 0 ]; then exit=1; fi
    log "$status" "$3" #by convention the third arg contains the URL
    return $exit
  fi
  global_dns_error_cnt=0
  return 0
}

# number of retries to try not to last more than default terminateGracePeriodSeconds (0 + 1 + 2 + 4 + 8 + 16 + 32 + 64 < 180s)
retries_count=8

function retry() {
  local retries=$1
  shift

  local count=0
  until "$@"; do
    exit=$?
    wait=$((2 ** count))
    count=$((count + 1))
    if [ $global_dns_error_cnt -gt "$max_dns_errors" ]; then
      error_exit "too many DNS errors, giving up"
    fi
    if [ $count -lt "$retries" ]; then
      log "retry $count/$retries exited $exit, retrying in $wait seconds"
      sleep $wait
    else
      log "retry $count/$retries exited $exit, no more retries left"
      return $exit
    fi
  done
  return 0
}

function log() {
   local timestamp
   timestamp=$(date --iso-8601=seconds)
   echo "{\"@timestamp\": \"${timestamp}\", \"message\": \"$*\", \"ecs.version\": \"1.2.0\", \"event.dataset\": \"elasticsearch.pre-stop-hook\"}" | tee /proc/1/fd/2 2> /dev/null
}

function error_exit() {
  log "$*"
  delayed_exit 1
}

function delayed_exit() {
  local elapsed
  elapsed=$(duration "${script_start}")
  local remaining=$((PRE_STOP_ADDITIONAL_WAIT_SECONDS - elapsed))
  if (( remaining < 0 )); then
    exit "${1-0}"
  fi
  log "delaying termination for ${remaining} seconds"
  sleep $remaining
  exit "${1-0}"
}

function supports_node_shutdown() {
  local version="$1"
  version="${version#[vV]}"
  major="${version%%\.*}"
  minor="${version#*.}"
  minor="${minor%.*}"
  patch="${version##*.}"
  # node shutdown is supported as of 7.15.2
  if [ "$major" -lt 7 ]  || { [ "$major" -eq 7 ] && [ "$minor" -eq 15 ] && [ "$patch" -lt 2 ]; }; then
    return 1
  fi
  return 0
}

version=""
if [[ -f "/mnt/elastic-internal/downward-api/labels" ]]; then
  # get Elasticsearch version from the downward API
  version=$(grep "elasticsearch.k8s.elastic.co/version" "/mnt/elastic-internal/downward-api/labels" | cut -d '=' -f 2)
  # remove quotes
  version=$(echo "${version}" | tr -d '"')
fi

# if ES version does not support node shutdown exit early
if ! supports_node_shutdown "$version"; then
  delayed_exit
fi

# setup basic auth if credentials are available
if [ -f "/mnt/elastic-internal/pod-mounted-users/elastic-internal-pre-stop" ]; then
  PROBE_PASSWORD=$(<"/mnt/elastic-internal/pod-mounted-users/elastic-internal-pre-stop")
  BASIC_AUTH=("-u" "elastic-internal-pre-stop:${PROBE_PASSWORD}")
else
  # typically the case on upgrades from versions that did not have this script yet and the necessary volume mounts are missing
  log "no API credentials available, will not attempt node shutdown orchestration from pre-stop hook"
  delayed_exit
fi

ES_URL="https://test-es-http.default.svc:9200"

log "retrieving node ID"
if ! retry "$retries_count" request -X GET "${ES_URL}/_cat/nodes?full_id=true&h=id,name" "${BASIC_AUTH[@]}"
then
  error_exit "failed to retrieve nodes"
fi

if ! NODE_ID="$(grep "${POD_NAME}" "${resp_body}" | cut -f 1 -d ' ')"
then
  error_exit "failed to extract node id"
fi

# check if there is an ongoing shutdown request
if ! request -X GET "${ES_URL}/_nodes/${NODE_ID}/shutdown" "${BASIC_AUTH[@]}"
then
  error_exit "failed to retrieve shutdown status"
fi

if grep -q -v '"nodes":\[\]' "$resp_body"; then
  log "shutdown managed by ECK operator"
  delayed_exit
fi

log "initiating node shutdown"
if ! retry "$retries_count" request -X PUT "${ES_URL}/_nodes/${NODE_ID}/shutdown" "${BASIC_AUTH[@]}" -H 'Content-Type: application/json' -d"
{
  \"type\": \"${shutdown_type}\",
  \"reason\": \"pre-stop hook\"
}"
then
  error_exit "failed to call node shutdown API"
fi

while :
do
  log "waiting for node shutdown to complete"
  if request -X GET "${ES_URL}/_nodes/${NODE_ID}/shutdown" "${BASIC_AUTH[@]}" &&
    grep -q -v 'IN_PROGRESS\|STALLED' "$resp_body"
  then
    break
  fi
  sleep 10
done

delayed_exit

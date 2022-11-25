// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nodespec

import (
	"path"

	v1 "k8s.io/api/core/v1"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/http"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/volume"
)

func NewPreStopHook() *v1.LifecycleHandler {
	return &v1.LifecycleHandler{
		Exec: &v1.ExecAction{
			Command: []string{"bash", "-c", path.Join(volume.ScriptsVolumeMountPath, PreStopHookScriptConfigKey)}},
	}
}

const PreStopHookScriptConfigKey = "pre-stop-hook-script.sh"
const PreStopHookScript = `#!/usr/bin/env bash

set -euo pipefail

function flush {
  FLUSH_TIMEOUT=30
  
  # setup basic auth if credentials are available
  if [ -n "${LIFECYCLE_HOOK_USERNAME}" ] && [ -f "${LIFECYCLE_HOOK_PASSWORD_PATH}" ]; then
    LIFECYCLE_HOOK_PASSWORD=$(<${LIFECYCLE_HOOK_PASSWORD_PATH})
    BASIC_AUTH="-u ${LIFECYCLE_HOOK_USERNAME}:${LIFECYCLE_HOOK_PASSWORD}"
  else
    BASIC_AUTH=''
  fi
  
  # Check if we are using IPv6
  if [[ $POD_IP =~ .*:.* ]]; then
    LOOPBACK="[::1]"
  else 
    LOOPBACK=127.0.0.1
  fi
  
  ENDPOINT="${LIFECYCLE_HOOK_PROTOCOL:-https}://${LOOPBACK}:9200/_flush"
  ORIGIN_HEADER="` + http.InternalProductRequestHeaderString + `"
  echo "Performing a flush..."
  curl -o /dev/null --max-time ${FLUSH_TIMEOUT} -H "${ORIGIN_HEADER}" -XPOST -g -s -k ${BASIC_AUTH} $ENDPOINT || true
}

PRE_STOP_ACTIONS=,${PRE_STOP_ACTIONS:=},

if echo $PRE_STOP_ACTIONS | grep -q ",flush,"; then
  flush
fi

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

sleep $PRE_STOP_ADDITIONAL_WAIT_SECONDS
`

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package nodespec

import (
	"path"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/volume"
	v1 "k8s.io/api/core/v1"
)

func NewPreStopHook() *v1.Handler {
	return &v1.Handler{
		Exec: &v1.ExecAction{
			Command: []string{"bash", "-c", path.Join(volume.ScriptsVolumeMountPath, PreStopHookScriptConfigKey)}},
	}
}

const PreStopHookScriptConfigKey = "pre-stop-hook-script.sh"
const PreStopHookScript = `#!/usr/bin/env bash

set -eux

# This script will wait for up to $PRE_STOP_MAX_WAIT_SECONDS for $POD_IP to disappear from DNS record,
# then it will wait additional $PRE_STOP_ADDITIONAL_WAIT_SECONDS and exit. This slows down the process shutdown
# and allows to make changes to the pool gracefully, without blackholing traffic when DNS
# contains IP that is already inactive. Assumes $HEADLESS_SERVICE_NAME and $POD_IP env variables are defined.

# Max time to wait for pods IP to disappear from DNS.
# As this runs in parallel to grace period after which process is SIGKILLed,
# it should be set to allow enough time for the process to gracefully terminate.
PRE_STOP_MAX_WAIT_SECONDS=${PRE_STOP_MAX_WAIT_SECONDS:=20}

# Additional wait before shutting down Elasticsearch.
# It allows kube-proxy to refresh its rules and remove the terminating Pod IP.
# Kube-proxy refresh period defaults to every 30 seconds, but the operation itself can take much longer if
# using iptables with a lot of services, in which case the default 30sec might not be enough.
# Also gives some additional bonus time to in-flight requests to terminate, and new requests to still
# target the Pod IP before Elasticsearch stops.
PRE_STOP_ADDITIONAL_WAIT_SECONDS=${PRE_STOP_ADDITIONAL_WAIT_SECONDS:=30}

START_TIME=$(date +%s)
while true; do
   ELAPSED_TIME=$(($(date +%s) - $START_TIME))

   if [ $ELAPSED_TIME -gt $PRE_STOP_MAX_WAIT_SECONDS ]; then
      exit 1
   fi

   if ! getent hosts $HEADLESS_SERVICE_NAME | grep $POD_IP; then
      sleep $PRE_STOP_ADDITIONAL_WAIT_SECONDS
      exit 0
   fi

   sleep 1
done
`

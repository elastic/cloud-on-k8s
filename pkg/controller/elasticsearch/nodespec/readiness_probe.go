// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package nodespec

import (
	"path"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/volume"
	corev1 "k8s.io/api/core/v1"
)

func NewReadinessProbe() *corev1.Probe {
	return &corev1.Probe{
		FailureThreshold:    3,
		InitialDelaySeconds: 10,
		PeriodSeconds:       5,
		SuccessThreshold:    1,
		TimeoutSeconds:      5,
		Handler: corev1.Handler{
			Exec: &corev1.ExecAction{
				Command: []string{"bash", "-c", path.Join(volume.ScriptsVolumeMountPath, ReadinessProbeScriptConfigKey)},
			},
		},
	}
}

const ReadinessProbeScriptConfigKey = "readiness-probe-script.sh"
const ReadinessProbeScript = `#!/usr/bin/env bash
# Consider a node to be healthy if it responds to a simple GET on "/_cat/nodes?local"
CURL_TIMEOUT=3

# Check if PROBE_PASSWORD_PATH is set, otherwise fall back to its former name in 1.0.0.beta-1: PROBE_PASSWORD_FILE
if [[ -z "${PROBE_PASSWORD_PATH}" ]]; then
  probe_password_path="${PROBE_PASSWORD_FILE}"
else
  probe_password_path="${PROBE_PASSWORD_PATH}"
fi

# setup basic auth if credentials are available
if [ -n "${PROBE_USERNAME}" ] && [ -f "${probe_password_path}" ]; then
  PROBE_PASSWORD=$(<${probe_password_path})
  BASIC_AUTH="-u ${PROBE_USERNAME}:${PROBE_PASSWORD}"
else
  BASIC_AUTH=''
fi

# request Elasticsearch
ENDPOINT="${READINESS_PROBE_PROTOCOL:-https}://127.0.0.1:9200/_cat/nodes?local"
status=$(curl -o /dev/null -w "%{http_code}" --max-time $CURL_TIMEOUT -XGET -s -k ${BASIC_AUTH} $ENDPOINT)

# ready if status code 200
if [[ $status == "200" ]]; then
	exit 0
else
	exit 1
fi
`

const PreStopHookScriptConfigKey = "pre-stop-hook-script.sh"
const PreStopHookScript = `#!/usr/bin/env bash

# This script will wait for up to $MAX_WAIT_SECONDS for $POD_IP to disappear from DNS record,
# then it will wait additional $ADDN_WAIT_SECONDS and exit. This slows down the process shutdown
# and allows to make changes to the pool gracefully, without blackholing traffic when DNS
# contains IP that is already inactive. Assumes $SERVICE_NAME and $POD_IP env variables are defined.

MAX_WAIT_SECONDS=20 # max time to wait for pods IP to disappear from DNS
ADDN_WAIT_SECONDS=1 # additional wait, allows queries to successfully use IP from old DNS entry

for i in {1..$MAX_WAIT_SECONDS}
do
   getent hosts $SERVICE_NAME | grep $POD_IP || sleep $ADDN_WAIT_SECONDS && exit 0
   sleep 1
done

exit 1
`

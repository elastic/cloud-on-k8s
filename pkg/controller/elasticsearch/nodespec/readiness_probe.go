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
const ReadinessProbeScript string = `
#!/usr/bin/env bash
# Consider a node to be healthy if it responds to a simple GET on "/"
CURL_TIMEOUT=3

# setup basic auth if credentials are available
if [ -n "${PROBE_USERNAME}" ] && [ -f "${PROBE_PASSWORD_FILE}" ]; then
  PROBE_PASSWORD=$(<$PROBE_PASSWORD_FILE)
  BASIC_AUTH="-u ${PROBE_USERNAME}:${PROBE_PASSWORD}"
else
  BASIC_AUTH=''
fi

# request Elasticsearch
status=$(curl -o /dev/null -w "%{http_code}" --max-time $CURL_TIMEOUT -XGET -s -k ${BASIC_AUTH} ${READINESS_PROBE_PROTOCOL:-https}://127.0.0.1:9200)

# ready if status code 200
if [[ $status == "200" ]]; then
	exit 0
else
	exit 1
fi
`

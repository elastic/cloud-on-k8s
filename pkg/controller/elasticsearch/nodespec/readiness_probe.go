// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package nodespec

import (
	"path"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
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

# fail should be called as a last resort to help the user to understand why the probe failed
function fail {
  timestamp=$(date --iso-8601=seconds)
  echo "{\"timestamp\": \"${timestamp}\", \"message\": \"readiness probe failed\", "$1"}" | tee /proc/1/fd/2 2> /dev/null
  exit 1
}

labels="` + volume.DownwardAPIMountPath + "/" + volume.LabelsFile + `"

version=""
if [[ -f "${labels}" ]]; then
  # get Elasticsearch version from the downward API
  version=$(grep "` + label.VersionLabelName + `" ${labels} | cut -d '=' -f 2)
  # remove quotes
  version=$(echo "${version}" | tr -d '"')
fi

READINESS_PROBE_TIMEOUT=${READINESS_PROBE_TIMEOUT:=3}

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

# request Elasticsearch on /
ENDPOINT="${READINESS_PROBE_PROTOCOL:-https}://127.0.0.1:9200/"
status=$(curl -o /dev/null -w "%{http_code}" --max-time ${READINESS_PROBE_TIMEOUT} -XGET -s -k ${BASIC_AUTH} $ENDPOINT)
curl_rc=$?

if [[ ${curl_rc} -ne 0 ]]; then
  fail "\"curl_rc\": \"${curl_rc}\""
fi

# ready if status code 200, 503 is tolerable if ES version is 6.x
if [[ ${status} == "200" ]] || [[ ${status} == "503" && ${version:0:2} == "6." ]]; then
  exit 0
else
  fail " \"status\": \"${status}\", \"version\":\"${version}\" "
fi
`

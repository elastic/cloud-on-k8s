// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nodespec

import (
	"bytes"
	"path"
	"text/template"

	v1 "k8s.io/api/core/v1"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/volume"
)

func NewPreStopHook() *v1.LifecycleHandler {
	return &v1.LifecycleHandler{
		Exec: &v1.ExecAction{
			Command: []string{"bash", "-c", path.Join(volume.ScriptsVolumeMountPath, PreStopHookScriptConfigKey)}},
	}
}

func RenderPreStopHookScript(esName string) (string, error) {
	buffer := bytes.Buffer{}
	tpl, err := template.New("").Parse(preStopHookScript)
	if err != nil {
		return "", err
	}
	if err := tpl.Execute(&buffer, map[string]string{"InternalServiceName": services.InternalServiceName(esName)}); err != nil {
		return "", err
	}
	return buffer.String(), nil
}

const PreStopHookScriptConfigKey = "pre-stop-hook-script.sh"
const preStopHookScript = `#!/usr/bin/env bash

set -euo pipefail

# This script will wait for up to $PRE_STOP_MAX_WAIT_SECONDS for the $POD_IP to disappear from the DNS record,
# then it will wait additional $PRE_STOP_ADDITIONAL_WAIT_SECONDS before allowing termination of the Pod.
# This slows down the process shutdown and allows to make changes to the pool gracefully, without blackholing traffic when DNS
# still contains the IP that is already inactive. 
# As this runs in parallel to grace period after which process is SIGKILLed,
# it should be set to allow enough time for the process to gracefully terminate.
# It allows kube-proxy to refresh its rules and remove the terminating Pod IP.
# Kube-proxy refresh period defaults to every 30 seconds, but the operation itself can take much longer if
# using iptables with a lot of services, in which case the default 30sec might not be enough.
# Also gives some additional bonus time to in-flight requests to terminate, and new requests to still
# target the Pod IP before Elasticsearch stops.

PRE_STOP_MAX_WAIT_SECONDS=${PRE_STOP_MAX_WAIT_SECONDS:=300}
PRE_STOP_ADDITIONAL_WAIT_SECONDS=${PRE_STOP_ADDITIONAL_WAIT_SECONDS:=50}
START_TIME=$(date +%s)
while true; do
   ELAPSED_TIME=$(($(date +%s) - $START_TIME))

   if [ $ELAPSED_TIME -gt $PRE_STOP_MAX_WAIT_SECONDS ]; then
      echo "timed out waiting for Pod IP removal from service"
      exit 1
   fi

   if ! getent hosts {{.InternalServiceName}} grep $POD_IP; then
      sleep $PRE_STOP_ADDITIONAL_WAIT_SECONDS
      exit 0
   fi
   sleep 1
done
`

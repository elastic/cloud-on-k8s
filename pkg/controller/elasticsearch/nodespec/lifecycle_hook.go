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

# This script will wait for up to $MAX_WAIT_SECONDS for $POD_IP to disappear from DNS record,
# then it will wait additional $ADDITIONAL_WAIT_SECONDS and exit. This slows down the process shutdown
# and allows to make changes to the pool gracefully, without blackholing traffic when DNS
# contains IP that is already inactive. Assumes $HEADLESS_SERVICE_NAME and $POD_IP env variables are defined.

# max time to wait for pods IP to disappear from DNS. As this runs in parallel to grace period
# (defaulting to 30s) after which process is SIGKILLed, it should be set to allow enough time
# for the process to gracefully terminate.
MAX_WAIT_SECONDS=20

# additional wait, allows queries to successfully use IP from DNS from before pod termination
# this gives a little bit more time for clients that resolved DNS just before DNS record
# was updated.
ADDITIONAL_WAIT_SECONDS=1

START_TIME=$(date +%s)
while true; do
   ELAPSED_TIME=$(($(date +%s) - $START_TIME))

   if [ $ELAPSED_TIME -gt $MAX_WAIT_SECONDS ]; then
      exit 1
   fi

   if ! getent hosts $HEADLESS_SERVICE_NAME | grep $POD_IP; then
      sleep $ADDITIONAL_WAIT_SECONDS
      exit 0
   fi

   sleep 1
done
`

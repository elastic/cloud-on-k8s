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
# then it will wait additional $ADDN_WAIT_SECONDS and exit. This slows down the process shutdown
# and allows to make changes to the pool gracefully, without blackholing traffic when DNS
# contains IP that is already inactive. Assumes $SERVICE_NAME and $POD_IP env variables are defined.

MAX_WAIT_SECONDS=20 # max time to wait for pods IP to disappear from DNS
ADDN_WAIT_SECONDS=1 # additional wait, allows queries to successfully use IP from DNS from before pod termination

for i in {1..$MAX_WAIT_SECONDS}
do
   getent hosts $SERVICE_NAME | grep $POD_IP || sleep $ADDN_WAIT_SECONDS && exit 0
   sleep 1
done

exit 1
`

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

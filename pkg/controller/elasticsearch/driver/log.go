// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	ulog "github.com/elastic/cloud-on-k8s/pkg/utils/log"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
)

var log = ulog.Log.WithName("driver")

func ssetLogger(statefulSet appsv1.StatefulSet) logr.Logger {
	return log.WithValues(
		"namespace", statefulSet.Namespace,
		"statefulset_name", statefulSet.Name,
	)
}

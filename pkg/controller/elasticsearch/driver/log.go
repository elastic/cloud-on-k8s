// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"context"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"

	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

func ssetLogger(ctx context.Context, statefulSet appsv1.StatefulSet) logr.Logger {
	return ulog.FromContext(ctx).WithValues(
		"namespace", statefulSet.Namespace,
		"statefulset_name", statefulSet.Name,
	)
}

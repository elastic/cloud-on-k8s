// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package annotation

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

// ExtractTimeout extracts a timeout value specified as an annotation on a resource.
func ExtractTimeout(ctx context.Context, objMeta metav1.ObjectMeta, annotation string, defaultVal time.Duration) time.Duration {
	if len(objMeta.Annotations) == 0 {
		return defaultVal
	}

	t, ok := objMeta.Annotations[annotation]
	if !ok {
		return defaultVal
	}

	timeout, err := time.ParseDuration(t)
	if err != nil {
		ulog.FromContext(ctx).Error(err, "Failed to parse timeout value from annotation", "annotation", annotation, "value", t)
		return defaultVal
	}

	return timeout
}

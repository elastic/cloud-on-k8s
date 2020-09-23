// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package annotation

import (
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
)

// ExtractTimeout extracts a timeout value specified as an annotation on a resource.
func ExtractTimeout(obj runtime.Object, annotation string, defaultVal time.Duration) time.Duration {
	if obj == nil {
		return defaultVal
	}

	metaAcc := meta.NewAccessor()

	ann, err := metaAcc.Annotations(obj)
	if err != nil {
		log.V(1).Info("Failed to extract annotations from object", "error", err, "object", obj)
		return defaultVal
	}

	if len(ann) == 0 {
		return defaultVal
	}

	t, ok := ann[annotation]
	if !ok {
		return defaultVal
	}

	timeout, err := time.ParseDuration(t)
	if err != nil {
		log.Error(err, "Failed to parse timeout value from annotation", "annotation", annotation, "value", t)
		return defaultVal
	}

	return timeout
}

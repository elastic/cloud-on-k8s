// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package annotation

import (
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const (
	UpdateAnnotation = "update.k8s.elastic.co/timestamp"
)

var (
	log = logf.Log.WithName("annotation")
)

// MarkPodAsUpdated updates a specific annotation on the pod, it is mostly used as a convenient method
// to speedup secret propagation into the pod.
// This is done as a best effort, some pods may not be updated, errors are only logged.
// This could be fixed in kubelet at some point, see https://github.com/kubernetes/kubernetes/issues/30189
func MarkPodAsUpdated(
	c k8s.Client,
	pod corev1.Pod,
) {
	log.V(1).Info(
		"Update annotation on pod",
		"annotation", UpdateAnnotation,
		"namespace", pod.Namespace,
		"pod", pod.Name,
	)
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[UpdateAnnotation] =
		time.Now().Format(time.RFC3339Nano) // nano should be enough to avoid collisions and keep it readable by a human.
	if err := c.Update(&pod); err != nil {
		log.Error(
			err,
			"failed to update annotation on pod",
			"annotation", UpdateAnnotation,
			"namespace", pod.Namespace,
			"pod", pod.Name,
		)
	}
}

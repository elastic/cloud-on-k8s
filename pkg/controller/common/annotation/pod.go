// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package annotation

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

// MarkPodsAsUpdated updates a specific annotation on the pods to speedup secret propagation.
func MarkPodsAsUpdated(
	ctx context.Context,
	c k8s.Client,
	podListOptions ...client.ListOption,
) {
	// Get all pods
	var podList corev1.PodList
	err := c.List(ctx, &podList, podListOptions...)
	if err != nil {
		ulog.FromContext(ctx).Error(err, "failed to list pods for annotation update")
		return
	}
	// Update annotation
	for _, pod := range podList.Items {
		MarkPodAsUpdated(ctx, c, pod)
	}
}

// MarkPodAsUpdated updates a specific annotation on the pod, it is mostly used as a convenient method
// to speedup secret propagation into the pod.
// This is done as a best effort, some pods may not be updated, errors are only logged.
// This could be fixed in kubelet at some point, see https://github.com/kubernetes/kubernetes/issues/30189
func MarkPodAsUpdated(
	ctx context.Context,
	c k8s.Client,
	pod corev1.Pod,
) {
	log := ulog.FromContext(ctx)
	log.V(1).Info(
		"Updating annotation on pod",
		"annotation", UpdateAnnotation,
		"namespace", pod.Namespace,
		"pod_name", pod.Name,
	)
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[UpdateAnnotation] =
		time.Now().Format(time.RFC3339Nano) // nano should be enough to avoid collisions and keep it readable by a human.
	if err := c.Update(ctx, &pod); err != nil {
		if errors.IsConflict(err) {
			// Conflicts are expected and will be handled on the next reconcile loop, no need to error out here
			log.V(1).Info("Conflict while updating pod annotation", "namespace", pod.Namespace, "pod_name", pod.Name)
		} else {
			log.Error(err, "failed to update pod annotation",
				"annotation", UpdateAnnotation,
				"namespace", pod.Namespace,
				"pod_name", pod.Name)
		}
	}
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package k8s

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const (
	UpdateAnnotation = "update.k8s.elastic.co/timestamp"
)

var (
	log = logf.Log.WithName("k8sutils")
)

// ToObjectMeta returns an ObjectMeta based on the given NamespacedName.
func ToObjectMeta(namespacedName types.NamespacedName) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Namespace: namespacedName.Namespace,
		Name:      namespacedName.Name,
	}
}

// ExtractNamespacedName returns an NamespacedName based on the given Object.
func ExtractNamespacedName(object metav1.Object) types.NamespacedName {
	return types.NamespacedName{
		Namespace: object.GetNamespace(),
		Name:      object.GetName(),
	}
}

// MarkPodAsUpdated updates a specific annotation on the pod, it is mostly used as a convenient method
// to speedup secret propagation into the pod.
// This is done as a best effort, some pods may not be updated, errors are only logged.
// see https://github.com/kubernetes/kubernetes/issues/30189
func MarkPodAsUpdated(
	c Client,
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

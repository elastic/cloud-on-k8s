// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package k8s

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

// IsAvailable checks if both conditions ContainersReady and PodReady of a Pod are true.
func IsPodReady(pod corev1.Pod) bool {
	conditionsTrue := 0
	for _, cond := range pod.Status.Conditions {
		if cond.Status == corev1.ConditionTrue && (cond.Type == corev1.ContainersReady || cond.Type == corev1.PodReady) {
			conditionsTrue++
		}
	}
	return conditionsTrue == 2
}

// GetPods returns the list of pods given a NamespacedName and a field selector.
func GetPods(
	c Client,
	namespace string,
	labelSelector labels.Selector,
	fieldSelector fields.Selector,
) ([]corev1.Pod, error) {
	var podList corev1.PodList

	listOpts := client.ListOptions{
		Namespace:     namespace,
		LabelSelector: labelSelector,
		FieldSelector: fieldSelector,
	}

	if err := c.List(&listOpts, &podList); err != nil {
		return nil, err
	}

	return podList.Items, nil
}

// GetServiceDNSName returns the fully qualified DNS name for a service
func GetServiceDNSName(svc corev1.Service) []string {
	return []string{
		fmt.Sprintf("%s.%s.svc", svc.Name, svc.Namespace),
		fmt.Sprintf("%s.%s", svc.Name, svc.Namespace),
	}
}

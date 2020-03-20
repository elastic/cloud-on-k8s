// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package k8s

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
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

// PodsByName returns a map of pod names to pods
func PodsByName(pods []corev1.Pod) map[string]corev1.Pod {
	podMap := make(map[string]corev1.Pod, len(pods))
	for _, pod := range pods {
		podMap[pod.Name] = pod
	}
	return podMap
}

// PodNames returns the names of the given pods.
func PodNames(pods []corev1.Pod) []string {
	names := make([]string, 0, len(pods))
	for _, pod := range pods {
		names = append(names, pod.Name)
	}
	return names
}

// GetServiceDNSName returns the fully qualified DNS name for a service
func GetServiceDNSName(svc corev1.Service) []string {
	return []string{
		fmt.Sprintf("%s.%s.svc", svc.Name, svc.Namespace),
		fmt.Sprintf("%s.%s", svc.Name, svc.Namespace),
	}
}

// EmitErrorEvent emits an event if the error is report-worthy
func EmitErrorEvent(r record.EventRecorder, err error, obj runtime.Object, reason, message string, args ...interface{}) {
	// ignore nil errors and conflict issues
	if err == nil || errors.IsConflict(err) {
		return
	}

	r.Eventf(obj, corev1.EventTypeWarning, reason, message, args...)
}

// GetSecretEntry returns the value of the secret data for the given key, or nil.
func GetSecretEntry(secret corev1.Secret, key string) []byte {
	if secret.Data == nil {
		return nil
	}
	content, exists := secret.Data[key]
	if !exists {
		return nil
	}
	return content
}

// DeleteSecretMatching deletes the Secret matching the provided selectors.
func DeleteSecretMatching(c Client, opts ...client.ListOption) error {
	var secrets corev1.SecretList
	if err := c.List(&secrets, opts...); err != nil {
		return err
	}
	for _, s := range secrets.Items {
		if err := c.Delete(&s); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

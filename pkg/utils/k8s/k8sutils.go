// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package k8s

import (
	"context"
	"fmt"
	"net"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	netutil "github.com/elastic/cloud-on-k8s/pkg/utils/net"
)

// DeepCopyObject creates a deep copy of a client.Object.
// This is to get around the limitation of the DeepCopyObject method which returns a runtime.Object.
func DeepCopyObject(obj client.Object) client.Object {
	if obj == nil {
		return nil
	}

	if newObj := obj.DeepCopyObject(); newObj != nil {
		return newObj.(client.Object)
	}

	return nil
}

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

// ObjectExists returns true if the object pointed by ref exists.
// typedReceiver acts as a generic object but must be of the desired object underlying type.
func ObjectExists(c Client, ref types.NamespacedName, typedReceiver client.Object) (bool, error) {
	err := c.Get(context.Background(), ref, typedReceiver)
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
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

// GetServiceDNSName returns the fully qualified DNS name for a service along with any external names provided by ingresses.
func GetServiceDNSName(svc corev1.Service) []string {
	names := []string{
		fmt.Sprintf("%s.%s.svc", svc.Name, svc.Namespace),
		fmt.Sprintf("%s.%s", svc.Name, svc.Namespace),
	}

	if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		for _, ingress := range svc.Status.LoadBalancer.Ingress {
			if ingress.Hostname != "" {
				names = append(names, ingress.Hostname)
			}
		}
	}

	return names
}

func GetServiceIPAddresses(svc corev1.Service) []net.IP {
	var ipAddrs []net.IP

	if len(svc.Spec.ExternalIPs) > 0 {
		ipAddrs = make([]net.IP, len(svc.Spec.ExternalIPs))
		for i, externalIP := range svc.Spec.ExternalIPs {
			ipAddrs[i] = netutil.IPToRFCForm(net.ParseIP(externalIP))
		}
	}

	if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		for _, ingress := range svc.Status.LoadBalancer.Ingress {
			if ingress.IP != "" {
				ipAddrs = append(ipAddrs, netutil.IPToRFCForm(net.ParseIP(ingress.IP))) //nolint:makezero
			}
		}
	}

	return ipAddrs
}

// EmitErrorEvent emits an event if the error is report-worthy
func EmitErrorEvent(r record.EventRecorder, err error, obj runtime.Object, reason, message string, args ...interface{}) {
	// ignore nil errors and conflict issues
	if err == nil || apierrors.IsConflict(err) {
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
	if err := c.List(context.Background(), &secrets, opts...); err != nil {
		return err
	}
	for _, s := range secrets.Items {
		secret := s
		if err := c.Delete(context.Background(), &secret); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// DeleteSecretIfExists deletes the secret identified by key if exists.
func DeleteSecretIfExists(c Client, key types.NamespacedName) error {
	var secret corev1.Secret
	err := c.Get(context.Background(), key, &secret)
	if err != nil && apierrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}
	err = c.Delete(context.Background(), &secret)
	if err != nil && apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

// PodsMatchingLabels returns Pods from the given namespace matching the given labels.
func PodsMatchingLabels(c Client, namespace string, labels map[string]string) ([]corev1.Pod, error) {
	var pods corev1.PodList
	if err := c.List(context.Background(), &pods, client.InNamespace(namespace), client.MatchingLabels(labels)); err != nil {
		return nil, err
	}
	return pods.Items, nil
}

func indexOfCtrlRef(owners []metav1.OwnerReference) int {
	for index, r := range owners {
		if r.Controller != nil && *r.Controller {
			return index
		}
	}
	return -1
}

type StorageComparison struct {
	Increase bool
	Decrease bool
}

// CompareStorageRequests compares storage requests in the given resource requirements.
// It returns a zero-ed StorageComparison in case one of the requests is zero (value not set: comparison not possible).
func CompareStorageRequests(initial corev1.ResourceRequirements, updated corev1.ResourceRequirements) StorageComparison {
	initialSize := initial.Requests.Storage()
	updatedSize := updated.Requests.Storage()
	if initialSize.IsZero() || updatedSize.IsZero() {
		return StorageComparison{}
	}
	switch updatedSize.Cmp(*initialSize) {
	case -1: // decrease
		return StorageComparison{Decrease: true}
	case 1: // increase
		return StorageComparison{Increase: true}
	default: // same size
		return StorageComparison{}
	}
}

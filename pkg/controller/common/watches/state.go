// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package watches

import (
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// NewDynamicWatches creates an initialized DynamicWatches container.
func NewDynamicWatches() DynamicWatches {
	return DynamicWatches{
		ConfigMaps:          NewDynamicEnqueueRequest[*corev1.ConfigMap, reconcile.Request](),
		Secrets:             NewDynamicEnqueueRequest[*corev1.Secret, reconcile.Request](),
		Services:            NewDynamicEnqueueRequest[*corev1.Service, reconcile.Request](),
		Pods:                NewDynamicEnqueueRequest[*corev1.Pod, reconcile.Request](),
		ReferencedResources: NewDynamicEnqueueRequest[client.Object, reconcile.Request](),
	}
}

// DynamicWatches contains stateful dynamic watches. Intended as facility to pass around stateful dynamic watches and
// give each of them an identity.
type DynamicWatches struct {
	ConfigMaps          *DynamicEnqueueRequest[*corev1.ConfigMap, reconcile.Request]
	Secrets             *DynamicEnqueueRequest[*corev1.Secret, reconcile.Request]
	Services            *DynamicEnqueueRequest[*corev1.Service, reconcile.Request]
	Pods                *DynamicEnqueueRequest[*corev1.Pod, reconcile.Request]
	ReferencedResources *DynamicEnqueueRequest[client.Object, reconcile.Request]
}

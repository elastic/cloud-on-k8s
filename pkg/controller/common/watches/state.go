// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package watches

import (
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Values of the "handler" label of elastic_dynamic_watches_registered.
const (
	configMapsHandlerKind          = "configmaps"
	secretsHandlerKind             = "secrets"
	servicesHandlerKind            = "services"
	podsHandlerKind                = "pods"
	referencedResourcesHandlerKind = "referencedresources"
	unknownHandlerKind             = "unknown"
)

// handlerKindFor returns the "handler" label value for the watched type T.
func handlerKindFor[T client.Object]() string {
	var zero T
	switch any(zero).(type) {
	case *corev1.ConfigMap:
		return configMapsHandlerKind
	case *corev1.Secret:
		return secretsHandlerKind
	case *corev1.Service:
		return servicesHandlerKind
	case *corev1.Pod:
		return podsHandlerKind
	case nil:
		// client.Object is used for the generic referenced-resources watch.
		return referencedResourcesHandlerKind
	default:
		return unknownHandlerKind
	}
}

// NewDynamicWatches creates an initialized DynamicWatches container.
func NewDynamicWatches() DynamicWatches {
	return DynamicWatches{
		ConfigMaps:          NewDynamicEnqueueRequest[*corev1.ConfigMap](),
		Secrets:             NewDynamicEnqueueRequest[*corev1.Secret](),
		Services:            NewDynamicEnqueueRequest[*corev1.Service](),
		Pods:                NewDynamicEnqueueRequest[*corev1.Pod](),
		ReferencedResources: NewDynamicEnqueueRequest[client.Object](),
	}
}

// DynamicWatches contains stateful dynamic watches. Intended as facility to pass around stateful dynamic watches and
// give each of them an identity.
type DynamicWatches struct {
	ConfigMaps          *DynamicEnqueueRequest[*corev1.ConfigMap]
	Secrets             *DynamicEnqueueRequest[*corev1.Secret]
	Services            *DynamicEnqueueRequest[*corev1.Service]
	Pods                *DynamicEnqueueRequest[*corev1.Pod]
	ReferencedResources *DynamicEnqueueRequest[client.Object]
}

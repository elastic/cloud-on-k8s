// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package annotation

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

// HasClientAuthenticationRequired returns true if the client-authentication-required annotation
// is present on the given object and set to "true".
func HasClientAuthenticationRequired(obj client.Object) bool {
	return obj.GetAnnotations()[ClientAuthenticationRequiredAnnotation] == "true"
}

// SetClientAuthenticationRequiredAnnotation sets the client-authentication-required annotation on the given resource.
// The annotation signals to association controllers that client certificates are needed.
// No-op if the annotation is already present.
func SetClientAuthenticationRequiredAnnotation(ctx context.Context, k8sClient k8s.Client, obj client.Object) error {
	if HasClientAuthenticationRequired(obj) {
		return nil // already set correctly
	}

	ulog.FromContext(ctx).Info(
		"Setting client-authentication-required annotation",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)

	existingAnnotations := obj.GetAnnotations()
	if existingAnnotations == nil {
		existingAnnotations = make(map[string]string)
	}
	existingAnnotations[ClientAuthenticationRequiredAnnotation] = "true"
	obj.SetAnnotations(existingAnnotations)
	return k8sClient.Update(ctx, obj)
}

// RemoveClientAuthenticationRequiredAnnotation removes the client-authentication-required annotation from the given resource.
// No-op if the annotation is already absent.
func RemoveClientAuthenticationRequiredAnnotation(ctx context.Context, k8sClient k8s.Client, obj client.Object) error {
	if !HasClientAuthenticationRequired(obj) {
		return nil // already set correctly
	}

	ulog.FromContext(ctx).Info(
		"Removing client-authentication-required annotation",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)

	existingAnnotations := obj.GetAnnotations()
	delete(existingAnnotations, ClientAuthenticationRequiredAnnotation)
	return k8sClient.Update(ctx, obj)
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

const (
	// ManagedAnnotation annotation
	LegacyPauseAnnoation = "common.k8s.elastic.co/pause"
	ManagedAnnotation    = "eck.k8s.elastic.co/managed"
)

// IsUnmanaged checks if a given resource is currently unmanaged.
func IsUnmanaged(ctx context.Context, object metav1.Object) bool {
	managed, exists := object.GetAnnotations()[ManagedAnnotation]
	if exists && managed == "false" {
		return true
	}

	paused, exists := object.GetAnnotations()[LegacyPauseAnnoation]
	if exists {
		ulog.FromContext(ctx).Info(fmt.Sprintf("%s is deprecated, please use %s", LegacyPauseAnnoation, ManagedAnnotation), "namespace", object.GetNamespace(), "name", object.GetName())
	}
	return exists && paused == "true"
}

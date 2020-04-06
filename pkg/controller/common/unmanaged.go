// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ManagedAnnotation annotation
	LegacyPauseAnnoation = "common.k8s.elastic.co/pause"
	ManagedAnnotation    = "eck.k8s.elastic.co/managed"
)

// IsUnmanaged checks if a given resource is currently unmanaged.
func IsUnmanaged(object metav1.Object) bool {
	managed, exists := object.GetAnnotations()[ManagedAnnotation]
	if exists && managed == "false" {
		return true
	}

	paused, exists := object.GetAnnotations()[LegacyPauseAnnoation]
	if exists {
		log.Info(fmt.Sprintf("%s is deprecated, please use %s", LegacyPauseAnnoation, ManagedAnnotation), "namespace", object.GetNamespace(), "name", object.GetName())
	}
	return exists && paused == "true"
}

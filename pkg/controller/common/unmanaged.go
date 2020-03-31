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
func IsUnmanaged(meta metav1.ObjectMeta) bool {
	managed, exists := meta.Annotations[ManagedAnnotation]
	if exists && managed == "false" {
		return true
	}

	paused, exists := meta.Annotations[LegacyPauseAnnoation]
	if exists {
		log.Info(fmt.Sprintf("%s is deprecated, please use %s", LegacyPauseAnnoation, ManagedAnnotation), "namespace", meta.Namespace, "name", meta.Name)
	}
	return exists && paused == "true"
}

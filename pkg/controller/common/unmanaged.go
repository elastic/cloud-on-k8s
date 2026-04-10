// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

const (
	LegacyPauseAnnoation = "common.k8s.elastic.co/pause"
	// ManagedAnnotation is the annotation set in order to signal to the operator to skip reconciliation entirely for
	// the given resource.
	//
	// Deprecated: Use PauseOrchestrationAnnotation instead. See https://github.com/elastic/cloud-on-k8s/issues/9295.
	ManagedAnnotation = "eck.k8s.elastic.co/managed"
	// PauseOrchestrationAnnotation pauses spec-driven orchestration (rolling upgrades, StatefulSet spec changes, scale
	// up/down) while keeping housekeeping running (certificate rotation, unicast hosts, user/secret reconciliation,
	// health monitoring).
	PauseOrchestrationAnnotation = "eck.k8s.elastic.co/pause-orchestration"
)

// IsOrchestrationPaused returns true if the PauseOrchestrationAnnotation exists and is set to true on the given resource
// to denote whether non-critical orchestration steps should continue.
func IsOrchestrationPaused(object metav1.Object) bool {
	paused, exists := object.GetAnnotations()[PauseOrchestrationAnnotation]
	if exists && paused == "true" {
		return true
	}

	return false
}

// IsUnmanaged checks if a given resource is currently unmanaged.
//
// Deprecated: Migrate to IsOrchestrationPaused instead. See https://github.com/elastic/cloud-on-k8s/issues/9295.
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

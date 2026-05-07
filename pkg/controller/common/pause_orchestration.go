// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// PauseOrchestrationAnnotation pauses spec-driven orchestration (rolling upgrades, StatefulSet spec changes, scale
	// up/down) while keeping housekeeping running (certificate rotation, unicast hosts, user/secret reconciliation,
	// health monitoring).
	PauseOrchestrationAnnotation = "eck.k8s.elastic.co/pause-orchestration"
)

// IsOrchestrationPaused returns true if the PauseOrchestrationAnnotation exists and is set to true on the given resource
// when non-critical orchestration steps should be skipped.
func IsOrchestrationPaused(object metav1.Object) bool {
	return object.GetAnnotations()[PauseOrchestrationAnnotation] == "true"
}

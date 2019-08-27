// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"strconv"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// PauseAnnotationName annotation
	PauseAnnotationName = "common.k8s.elastic.co/pause"
)

var (

	// PauseRequeue is the default requeue result if controller is paused
	PauseRequeue = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}
)

// IsPaused computes if a given controller is paused.
func IsPaused(meta metav1.ObjectMeta) bool {
	return getBoolFromAnnotation(meta.Annotations)
}

// Extract the desired state from the map that contains annotations.
func getBoolFromAnnotation(annotations map[string]string) bool {
	if annotations == nil {
		return false
	}

	stateAsString, exists := annotations[PauseAnnotationName]

	if !exists {
		return false
	}

	expectedState, err := strconv.ParseBool(stateAsString)
	if err != nil {
		log.Error(err, "Cannot parse %s as a bool, defaulting to %s: \"false\"", annotations[PauseAnnotationName], PauseAnnotationName)
		return false
	}

	return expectedState
}

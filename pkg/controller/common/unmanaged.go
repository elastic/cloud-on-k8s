// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"fmt"
	"strconv"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// ManagedAnnotation annotation
	LegacyPauseAnnoation = "common.k8s.elastic.co/pause"
	ManagedAnnotation    = "eck.k8s.elastic.co/managed"
)

var (

	// CheckManagedRequeue is the default requeue to re-check if a resource is still unmanaged.
	CheckManagedRequeue = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}
)

// IsUnmanaged checks if a given resource is currently unmanaged.
func IsUnmanaged(meta metav1.ObjectMeta) bool {
	return getBoolFromAnnotation(meta.Annotations)
}

// Extract the desired state from the map that contains annotations.
func getBoolFromAnnotation(annotations map[string]string) bool {
	if annotations == nil {
		return false
	}

	parse := func(ann string, defaultValue bool) bool {
		stateAsString, exists := annotations[ann]
		if !exists {
			return defaultValue
		}

		expectedState, err := strconv.ParseBool(stateAsString)
		if err != nil {
			log.Error(err, fmt.Sprintf("Cannot parse %s=%s as a bool, defaulting to %t", ann, stateAsString, defaultValue))
			return defaultValue
		}
		return expectedState
	}

	_, set := annotations[ManagedAnnotation]
	if set {
		return !parse(ManagedAnnotation, true)
	}
	_, set = annotations[LegacyPauseAnnoation]
	if set {
		log.Info(fmt.Sprintf("Using legacy annotation %s. Please consider moving to %s", LegacyPauseAnnoation, ManagedAnnotation))
		return parse(LegacyPauseAnnoation, false)
	}
	return false
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build es || e2e

package es

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
)

// TestRestartTriggerAnnotationCausesRollingRestart creates an HA cluster, sets the
// restart-trigger annotation, and asserts that a rolling restart completes successfully
// (pods ready, cluster green, data intact).
func TestRestartTriggerAnnotationCausesRollingRestart(t *testing.T) {
	// set CPU requests and memory limits, so the desired nodes API is used during a restart
	resources := corev1.ResourceRequirements{
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceCPU: resource.MustParse("1"),
		},
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		},
	}

	initial := elasticsearch.NewBuilder("test-restart-annotation").
		WithESMasterDataNodes(3, resources)

	triggerValue := time.Now().UTC().Format(time.RFC3339)
	mutated := initial.DeepCopy().
		WithNoESTopology().
		WithESMasterDataNodes(3, resources).
		WithAnnotation(esv1.RestartAllocationDelayAnnotation, "20m").
		WithAnnotation(esv1.RestartTriggerAnnotation, triggerValue)

	RunESMutation(t, initial, mutated)
}

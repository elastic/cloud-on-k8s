// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build es || e2e
// +build es e2e

package es

import (
	"context"
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
)

// TestSettingObservedGeneration tests that es.Generation, and es.Status.ObservedGeneration are incremented, and kept in sync
// when the spec of the ES cluster changes.
func TestSettingObservedGeneration(t *testing.T) {
	podTemplate1 := elasticsearch.ESPodTemplate(elasticsearch.DefaultResources)
	podTemplate1.Annotations = map[string]string{"foo": "bar"}

	initial := elasticsearch.NewBuilder("test-es-generation").
		WithNodeSet(esv1.NodeSet{
			Name:        "default",
			Count:       1,
			PodTemplate: podTemplate1,
		}).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	podTemplate2 := elasticsearch.ESPodTemplate(elasticsearch.DefaultResources)
	podTemplate2.Annotations = map[string]string{"foo": "bar2"}
	mutated := initial.WithNoESTopology().
		WithNodeSet(esv1.NodeSet{
			Name:        "default",
			Count:       1,
			PodTemplate: podTemplate2,
		})

	k := test.NewK8sClientOrFatal()

	var initialGeneration, initialObservedGeneration int64
	var eventualES esv1.Elasticsearch

	test.StepList{}.
		WithSteps(initial.InitTestSteps(k)).
		WithSteps(initial.CreationTestSteps(k)).
		WithSteps(test.CheckTestSteps(initial, k)).
		WithStep(test.Step{
			Name: "Get initial generation",
			Test: test.Eventually(func() error {
				var createdES esv1.Elasticsearch
				if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&initial.Elasticsearch), &createdES); err != nil {
					return err
				}
				initialGeneration = createdES.Generation
				initialObservedGeneration = createdES.Status.ObservedGeneration
				return nil
			}),
		}).
		WithSteps(mutated.UpgradeTestSteps(k)).
		WithSteps(test.CheckTestSteps(initial, k)).
		WithSteps(test.StepList{
			{
				Name: "Get Mutated ES Cluster",
				Test: test.Eventually(func() error {
					if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&initial.Elasticsearch), &eventualES); err != nil {
						return err
					}
					return nil
				}),
			},
			{
				Name: "ES.Generation should have been incremented; ES.Status.ObservedGeneration should have been incremented; ES.Status.ObsservedGeneration should equal ES.Generation",
				Test: func(t *testing.T) {
					if eventualES.Generation < initialGeneration {
						t.Errorf("Generation of ES cluster should have been incremented, current: %d, previous: %d", eventualES.Generation, initialGeneration)
					}
					if eventualES.Status.ObservedGeneration < initialObservedGeneration {
						t.Errorf("Status.ObservedGeneration of ES cluster should have been incremented, current: %d, previous: %d", eventualES.Status.ObservedGeneration, initialObservedGeneration)
					}

					if eventualES.Status.ObservedGeneration != eventualES.Generation {
						t.Errorf("Status.ObservedGeneration of ES cluster should equal current generation, current: %d, observedGeneration: %d", eventualES.Generation, eventualES.Status.ObservedGeneration)
					}
				},
			},
		}).RunSequential(t)
}

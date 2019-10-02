// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package es

import (
	"fmt"
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	corev1 "k8s.io/api/core/v1"
)

func TestForceUpgradePendingPods(t *testing.T) {
	// create a cluster whose Pods will stay Pending forever
	initial := elasticsearch.NewBuilder("force-upgrade-pending").
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)
	initial.Elasticsearch.Spec.NodeSets[0].PodTemplate.Spec.NodeSelector = map[string]string{
		"cannot": "be-scheduled",
	}
	// fix that cluster to remove the wrong NodeSelector
	fixed := elasticsearch.Builder{}
	fixed.Elasticsearch = *initial.Elasticsearch.DeepCopy()
	fixed.Elasticsearch.Spec.NodeSets[0].PodTemplate.Spec.NodeSelector = nil

	k := test.NewK8sClientOrFatal()
	elasticsearch.ForcedUpgradeTestSteps(
		k,
		initial,
		// wait for all initial Pods to be Pending
		elasticsearch.CheckESPodsPending(initial, k),
		fixed,
	).RunSequential(t)
}

func TestForceUpgradeBootloopingPods(t *testing.T) {
	// create a cluster with a bad ES configuration that leads to Pods bootlooping
	initial := elasticsearch.NewBuilder("force-upgrade-bootloop").
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithAdditionalConfig(map[string]map[string]interface{}{
			"masterdata": {
				"this leads": "to a bootlooping instance",
			},
		})

	// fix that cluster to remove the wrong configuration
	fixed := elasticsearch.Builder{}
	fixed.Elasticsearch = *initial.Elasticsearch.DeepCopy()
	fixed.Elasticsearch.Spec.NodeSets[0].Config = nil

	k := test.NewK8sClientOrFatal()
	elasticsearch.ForcedUpgradeTestSteps(
		k,
		initial,
		// wait for Pods to restart due to wrong config
		elasticsearch.CheckPodsCondition(
			initial,
			k,
			"Pods should have restarted at least once due to wrong ES config",
			func(p corev1.Pod) error {
				for _, containerStatus := range p.Status.ContainerStatuses {
					if containerStatus.Name != v1alpha1.ElasticsearchContainerName {
						continue
					}
					if containerStatus.RestartCount < 1 {
						return fmt.Errorf("container not restarted yet")
					}
					return nil
				}
				return fmt.Errorf("container %s not found in pod %s", v1alpha1.ElasticsearchContainerName, p.Name)
			},
		),
		fixed,
	).RunSequential(t)
}

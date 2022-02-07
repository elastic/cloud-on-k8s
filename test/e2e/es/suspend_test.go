// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// +build es e2e

package es

import (
	"context"
	"fmt"
	"strings"
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	corev1 "k8s.io/api/core/v1"
)

func TestESSuspendPod(t *testing.T) {
	esName := "test-suspend-pod"

	// Create a multi-node cluster
	builder := elasticsearch.NewBuilder(esName).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	stepsFn := func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Suspend all Pods",
				Test: test.Eventually(func() error {
					pods, err := k.GetPods(test.ESPodListOptions(builder.Elasticsearch.Namespace, builder.Elasticsearch.Name)...)
					if err != nil {
						return err
					}
					var podNames []string
					for _, p := range pods {
						podNames = append(podNames, p.Name)
					}
					var es esv1.Elasticsearch

					if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&builder.Elasticsearch), &es); err != nil {
						return err
					}
					es.Annotations[esv1.SuspendAnnotation] = strings.Join(podNames, ",")
					return k.Client.Update(context.Background(), &es)
				}),
			},
			elasticsearch.CheckPodsCondition(builder, k, "all Pods should be suspended", func(p corev1.Pod) error {
				nok := fmt.Errorf("pod %s/%s not suspended", p.Namespace, p.Name)
				for _, s := range p.Status.ContainerStatuses {
					// main container still ready? NOK
					if s.Ready {
						return nok
					}
				}
				for _, s := range p.Status.InitContainerStatuses {
					if s.Name == initcontainer.SuspendContainerName && s.State.Running != nil {
						return nil
					}
				}
				// suspend container not in running state? NOK
				return nok
			}),
			{
				Name: "Remove the suspend annotation",
				Test: test.Eventually(func() error {
					var es esv1.Elasticsearch
					if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&builder.Elasticsearch), &es); err != nil {
						return err
					}
					delete(es.Annotations, esv1.SuspendAnnotation)
					return k.Client.Update(context.Background(), &es)
				}),
			},
			// Pods should become ready again
			elasticsearch.CheckExpectedPodsEventuallyReady(builder, k),
		}.
			// Internal view of the cluster should also be as originally specified
			WithSteps(builder.CheckStackTestSteps(k))
	}
	test.Sequence(nil, stepsFn, builder).RunSequential(t)
}

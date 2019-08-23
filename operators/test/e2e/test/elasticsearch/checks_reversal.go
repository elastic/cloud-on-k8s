// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"errors"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test"
	"github.com/stretchr/testify/require"
)

func (b Builder) MutationReversalTestContext() test.ReversalTestContext {
	return &mutationReversalTestContext{
		es:                     b.Elasticsearch,
		initialRevisions:       make(map[string]string),
		initialCurrentReplicas: make(map[string]int32),
	}
}

type mutationReversalTestContext struct {
	es                     v1alpha1.Elasticsearch
	initialCurrentReplicas map[string]int32
	initialRevisions       map[string]string
	dataIntegrity          *DataIntegrityCheck
}

func (s *mutationReversalTestContext) PreMutationSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Remember the current config revisions",
			Test: func(t *testing.T) {
				statefulSets, err := sset.RetrieveActualStatefulSets(k.Client, k8s.ExtractNamespacedName(&s.es))
				require.NoError(t, err)
				for _, set := range statefulSets {
					s.initialRevisions[set.Name] = set.Status.CurrentRevision
					s.initialCurrentReplicas[set.Name] = set.Status.CurrentReplicas
				}
			},
		},
		{
			Name: "Add some data to the cluster before any mutation",
			Test: func(t *testing.T) {
				var err error
				s.dataIntegrity, err = NewDataIntegrityCheck(s.es, k)
				require.NoError(t, err)
				require.NoError(t, s.dataIntegrity.Init())
			},
		},
	}
}

func (s *mutationReversalTestContext) PostMutationSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Verify that a config change is being applied",
			Test: test.Eventually(func() error {
				statefulSets, err := sset.RetrieveActualStatefulSets(k.Client, k8s.ExtractNamespacedName(&s.es))
				if err != nil {
					return err
				}
				// at least one sset should have started replacing pods
				for _, set := range statefulSets {
					// case 1 simple scaling w/o config change
					if s.initialCurrentReplicas[set.Name] != set.Status.CurrentReplicas ||
						// case 2 actual config change which will also affect current replicas but this is supposed to
						// protect against us missing that if it happens too fast
						s.initialRevisions[set.Name] != set.Status.UpdateRevision && set.Status.UpdatedReplicas > 0 {
						return nil
					}
				}
				return errors.New("no upgrade in progress")
			}),
		},
	}
}

func (s *mutationReversalTestContext) VerificationSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Verify no data loss has happened during the aborted upgrade",
			Test: func(t *testing.T) {
				require.NoError(t, s.dataIntegrity.Verify())
			},
		},
	}
}

var _ test.ReversalTestContext = &mutationReversalTestContext{}

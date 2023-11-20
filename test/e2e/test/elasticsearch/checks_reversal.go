// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
)

// MutationReversalTestContext returns a context struct to test changes on a resource that are immediately reverted.
// We assume the resource to be ready and running.
// We assume the resource to be the same as the original resource after reversion.
func (b Builder) MutationReversalTestContext() *MutationReversalTestContext {
	return &MutationReversalTestContext{
		esBuilder:              b,
		initialRevisions:       make(map[string]string),
		initialCurrentReplicas: make(map[string]int32),
	}
}

type MutationReversalTestContext struct {
	esBuilder              Builder
	initialCurrentReplicas map[string]int32
	initialRevisions       map[string]string
	dataIntegrity          *DataIntegrityCheck
}

func (s *MutationReversalTestContext) PreMutationSteps(k *test.K8sClient) test.StepList {
	//nolint:thelper
	return test.StepList{
		{
			Name: "Remember the current config revisions",
			Test: func(t *testing.T) {
				statefulSets, err := sset.RetrieveActualStatefulSets(k.Client, k8s.ExtractNamespacedName(&s.esBuilder.Elasticsearch))
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
				s.dataIntegrity = NewDataIntegrityCheck(k, s.esBuilder)
				require.NoError(t, s.dataIntegrity.Init())
			},
		},
	}
}

func (s *MutationReversalTestContext) PostMutationSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Verify that a config change is being applied",
			Test: test.Eventually(func() error {
				statefulSets, err := sset.RetrieveActualStatefulSets(k.Client, k8s.ExtractNamespacedName(&s.esBuilder.Elasticsearch))
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

func (s *MutationReversalTestContext) VerificationSteps(_ *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Verify no data loss has happened during the aborted upgrade",
			Test: test.Eventually(func() error {
				return s.dataIntegrity.Verify()
			}),
		},
	}
}

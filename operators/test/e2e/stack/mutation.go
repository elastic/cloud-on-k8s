// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stack

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	estype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	esclient "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const continousHealthCheckTimeout = 25 * time.Second

type MutationTestsOptions struct {
	ExpectedNewPods  int
	ExpectedPVCReuse int
}

// MutationTestSteps tests topology changes on the given stack
// we expect the stack to be already created and running.
// If the stack to mutate to is the same as the original stack,
// then all tests should still pass.
func MutationTestSteps(stack Builder, k *helpers.K8sHelper, options MutationTestsOptions) []helpers.TestStep {

	var clusterIDBeforeMutation string
	initialPods := map[string]time.Time{}
	initialPVCs := map[string]struct{}{}

	var continuousHealthChecks *ContinousHealthCheck

	return helpers.TestStepList{}.
		WithSteps(
			helpers.TestStep{
				Name: "Retrieve pods and PVC names",
				Test: func(t *testing.T) {
					pods, err := k.GetPods(helpers.ESPodListOptions(stack.Elasticsearch.Name))
					require.NoError(t, err)
					for _, p := range pods {
						initialPods[p.Name] = p.CreationTimestamp.Time
						claimName := helpers.GetESDataVolumeClaimName(p)
						if claimName != "" {
							initialPVCs[claimName] = struct{}{}
						}
					}
				},
			},
			helpers.TestStep{
				Name: "Start querying ES cluster health while mutation is going on",
				Test: func(t *testing.T) {
					var err error
					continuousHealthChecks, err = NewContinousHealthCheck(stack.Elasticsearch, k)
					require.NoError(t, err)
					continuousHealthChecks.Start()
				},
			},
			RetrieveClusterUUIDStep(stack.Elasticsearch, k, &clusterIDBeforeMutation),
			helpers.TestStep{
				Name: "Applying the mutation should succeed",
				Test: func(t *testing.T) {
					var curEs estype.Elasticsearch
					require.NoError(t, k.Client.Get(k8s.ExtractNamespacedName(&stack.Elasticsearch), &curEs))
					curEs.Spec = stack.Elasticsearch.Spec
					require.NoError(t, k.Client.Update(&curEs))

					var curKb v1alpha1.Kibana
					require.NoError(t, k.Client.Get(k8s.ExtractNamespacedName(&stack.Kibana), &curKb))
					curKb.Spec = stack.Kibana.Spec
					require.NoError(t, k.Client.Update(&curKb))
				},
			},
			helpers.TestStep{
				Name: "The expected number of new pods should have been created, with the expected number of PVC reused",
				Test: helpers.Eventually(func() error {
					pods, err := k.GetPods(helpers.ESPodListOptions(stack.Elasticsearch.Name))
					if err != nil {
						return err
					}
					// get number of pods that were created or re-created reusing a PVC
					newPods := 0
					pvcReuse := 0
					for _, p := range pods {
						initialCreationDate, existed := initialPods[p.Name]
						switch {
						case !existed:
							// this pod is new
							newPods++

						case existed && !p.CreationTimestamp.Time.Equal(initialCreationDate):
							// this pod was recreated, probably reusing PVCs
							newPods++
							claimName := helpers.GetESDataVolumeClaimName(p)
							if claimName != "" {
								if _, existed := initialPVCs[claimName]; existed {
									// an existing PVC was reused
									pvcReuse++
								}
							}

						default:
							// this pod wasn't modified
						}
					}
					if newPods != options.ExpectedNewPods {
						return fmt.Errorf("expected %d new pods, got %d", options.ExpectedNewPods, newPods)
					}
					if pvcReuse != options.ExpectedPVCReuse {
						return fmt.Errorf("expected %d PVC reuse, got %d", options.ExpectedPVCReuse, pvcReuse)
					}
					return nil
				}),
			},
		).
		WithSteps(CheckStackSteps(stack, k)...).
		WithSteps(
			CompareClusterUUIDStep(stack.Elasticsearch, k, &clusterIDBeforeMutation),
			helpers.TestStep{
				Name: "Cluster health should not have been red during mutation process",
				Test: func(t *testing.T) {
					continuousHealthChecks.Stop()
					assert.Equal(t, 0, continuousHealthChecks.FailureCount)
					for _, f := range continuousHealthChecks.Failures {
						t.Errorf("Cluster health check failure at %s: %s", f.timestamp, f.err.Error())
					}
				},
			},
		)
}

// ContinuousHealthCheckFailure represents a healthchechk failure
type ContinuousHealthCheckFailure struct {
	err       error
	timestamp time.Time
}

// ContinousHealthCheck continously runs health checks against Elasticsearch
// during the whole mutation process
type ContinousHealthCheck struct {
	SuccessCount int
	FailureCount int
	Failures     []ContinuousHealthCheckFailure
	stopChan     chan struct{}
	esClient     esclient.Client
}

// NewContinousHealthCheck sets up a ContinousHealthCheck struct
func NewContinousHealthCheck(es estype.Elasticsearch, k *helpers.K8sHelper) (*ContinousHealthCheck, error) {
	esClient, err := helpers.NewElasticsearchClient(es, k)
	if err != nil {
		return nil, err
	}
	return &ContinousHealthCheck{
		stopChan: make(chan struct{}),
		esClient: esClient,
	}, nil
}

// AppendErr sets the given error as a failure
func (hc *ContinousHealthCheck) AppendErr(err error) {
	hc.Failures = append(hc.Failures, ContinuousHealthCheckFailure{
		err:       err,
		timestamp: time.Now(),
	})
	hc.FailureCount++
}

// Start runs health checks in a goroutine, until stopped
func (hc *ContinousHealthCheck) Start() {
	go func() {
		ticker := time.NewTicker(helpers.DefaultRetryDelay)
		for {
			select {
			case <-hc.stopChan:
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), continousHealthCheckTimeout)
				defer cancel()
				health, err := hc.esClient.GetClusterHealth(ctx)
				if err != nil {
					// TODO: Temporarily account only red clusters, see https://github.com/elastic/cloud-on-k8s/issues/614
					//hc.AppendErr(err)
					continue
				}
				if estype.ElasticsearchHealth(health.Status) == estype.ElasticsearchRedHealth {
					hc.AppendErr(errors.New("cluster health red"))
					continue
				}
				hc.SuccessCount++
			}
		}
	}()
}

// Stop the health checks goroutine
func (hc *ContinousHealthCheck) Stop() {
	hc.stopChan <- struct{}{}
}

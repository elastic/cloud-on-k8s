// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"context"
	"errors"
	"testing"
	"time"

	estype "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	continuousHealthCheckTimeout = 5 * time.Second
	// clusterUnavailabilityThreshold is the accepted duration for the cluster to temporarily not respond to requests
	// (eg. during leader elections in the middle of a rolling upgrade)
	clusterUnavailabilityThreshold = 60 * time.Second
)

func (b Builder) UpgradeTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		test.Step{
			Name: "Applying the Elasticsearch mutation should succeed",
			Test: func(t *testing.T) {
				var curEs estype.Elasticsearch
				require.NoError(t, k.Client.Get(k8s.ExtractNamespacedName(&b.Elasticsearch), &curEs))
				curEs.Spec = b.Elasticsearch.Spec
				require.NoError(t, k.Client.Update(&curEs))
			},
		},
	}
}

func (b Builder) MutationTestSteps(k *test.K8sClient) test.StepList {
	var clusterIDBeforeMutation string
	var continuousHealthChecks *ContinuousHealthCheck
	var dataIntegrityCheck *DataIntegrityCheck
	var masterChangeBudgetCheck *MasterChangeBudgetCheck
	var changeBudgetCheck *ChangeBudgetCheck

	return test.StepList{
		test.Step{
			Name: "Add some data to the cluster before starting the mutation",
			Test: func(t *testing.T) {
				dataIntegrityCheck = NewDataIntegrityCheck(k, b)
				require.NoError(t, dataIntegrityCheck.Init())
			},
		},
		test.Step{
			Name: "Start querying Elasticsearch cluster health while mutation is going on",
			Skip: func() bool {
				// Don't monitor cluster health if we're doing a rolling upgrade of a one data node cluster.
				// The cluster will become unavailable at some point, then its health will be red
				// after the upgrade while shards are initializing.
				return IsOneDataNodeRollingUpgrade(b)
			},
			Test: func(t *testing.T) {
				var err error
				continuousHealthChecks, err = NewContinuousHealthCheck(b, k)
				require.NoError(t, err)
				continuousHealthChecks.Start()
			},
		},
		test.Step{
			Name: "Start tracking master additions and removals",
			Test: func(t *testing.T) {
				masterChangeBudgetCheck = NewMasterChangeBudgetCheck(b.Elasticsearch, 1*time.Second, k.Client)
				masterChangeBudgetCheck.Start()
			},
		},
		test.Step{
			Name: "Start tracking pod count",
			Test: func(t *testing.T) {
				changeBudgetCheck = NewChangeBudgetCheck(b.Elasticsearch, k.Client)
				changeBudgetCheck.Start()
			},
		},
		RetrieveClusterUUIDStep(b.Elasticsearch, k, &clusterIDBeforeMutation),
	}.
		WithSteps(b.UpgradeTestSteps(k)).
		WithSteps(b.CheckK8sTestSteps(k)).
		WithSteps(b.CheckStackTestSteps(k)).
		WithSteps(test.StepList{
			CompareClusterUUIDStep(b.Elasticsearch, k, &clusterIDBeforeMutation),
			test.Step{
				Name: "Master change budget must not have been exceeded",
				Test: func(t *testing.T) {
					masterChangeBudgetCheck.Stop()
					require.NoError(t, masterChangeBudgetCheck.Verify(1)) // fixed budget of 1 master node added/removed at a time
				},
			},
			test.Step{
				Name: "Pod count must not violate change budget",
				Test: func(t *testing.T) {
					changeBudgetCheck.Stop()
					if b.MutatedFrom != nil {
						require.NoError(t, changeBudgetCheck.Verify(b.MutatedFrom.Elasticsearch.Spec, b.Elasticsearch.Spec))
					}
				},
			},
			test.Step{
				Name: "Elasticsearch cluster health should not have been red during mutation process",
				Skip: func() bool {
					return IsOneDataNodeRollingUpgrade(b)
				},
				Test: func(t *testing.T) {
					continuousHealthChecks.Stop()
					assert.Equal(t, 0, continuousHealthChecks.FailureCount)
					for _, f := range continuousHealthChecks.Failures {
						t.Errorf("Elasticsearch cluster health check failure at %s: %s", f.timestamp, f.err.Error())
					}
				},
			},
			test.Step{
				Name: "Data added initially should still be present",
				Test: test.Eventually(func() error { // nolint
					return dataIntegrityCheck.Verify()
				}),
			},
		})
}

func IsOneDataNodeRollingUpgrade(b Builder) bool {
	if b.MutatedFrom == nil {
		return false
	}
	initial := b.MutatedFrom.Elasticsearch
	mutated := b.Elasticsearch
	// consider we're in the 1-node rolling upgrade scenario if we mutate
	// from one data node to one data node with the same name
	if MustNumDataNodes(initial) == 1 && MustNumDataNodes(mutated) == 1 &&
		initial.Spec.NodeSets[0].Name == mutated.Spec.NodeSets[0].Name {
		return true
	}
	return false
}

// ContinuousHealthCheckFailure represents an health check failure
type ContinuousHealthCheckFailure struct {
	err       error
	timestamp time.Time
}

// ContinuousHealthCheck continuously runs health checks against Elasticsearch
// during the whole mutation process
type ContinuousHealthCheck struct {
	b            Builder
	SuccessCount int
	FailureCount int
	Failures     []ContinuousHealthCheckFailure
	stopChan     chan struct{}
	esClient     esclient.Client
}

// NewContinuousHealthCheck sets up a ContinuousHealthCheck struct
func NewContinuousHealthCheck(b Builder, k *test.K8sClient) (*ContinuousHealthCheck, error) {
	esClient, err := NewElasticsearchClient(b.Elasticsearch, k)
	if err != nil {
		return nil, err
	}
	return &ContinuousHealthCheck{
		b:        b,
		stopChan: make(chan struct{}),
		esClient: esClient,
	}, nil
}

// AppendErr sets the given error as a failure
func (hc *ContinuousHealthCheck) AppendErr(err error) {
	hc.Failures = append(hc.Failures, ContinuousHealthCheckFailure{
		err:       err,
		timestamp: time.Now(),
	})
	hc.FailureCount++
}

// Start runs health checks in a goroutine, until stopped
func (hc *ContinuousHealthCheck) Start() {
	clusterUnavailability := clusterUnavailability{threshold: clusterUnavailabilityThreshold}
	go func() {
		ticker := time.NewTicker(test.DefaultRetryDelay)
		for {
			select {
			case <-hc.stopChan:
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), continuousHealthCheckTimeout)
				defer cancel()
				health, err := hc.esClient.GetClusterHealth(ctx)
				if err != nil {
					// Could not retrieve cluster health, can happen when the master node is killed
					// during a rolling upgrade. We allow it, unless it lasts for too long.
					clusterUnavailability.markUnavailable()
					if clusterUnavailability.hasExceededThreshold() {
						// cluster has been unavailable for too long
						hc.AppendErr(err)
					}
					continue
				}
				clusterUnavailability.markAvailable()
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
func (hc *ContinuousHealthCheck) Stop() {
	hc.stopChan <- struct{}{}
}

type clusterUnavailability struct {
	start     time.Time
	threshold time.Duration
}

func (cu *clusterUnavailability) markUnavailable() {
	if cu.start.IsZero() {
		cu.start = time.Now()
	}
}

func (cu *clusterUnavailability) markAvailable() {
	cu.start = time.Time{}
}

func (cu *clusterUnavailability) hasExceededThreshold() bool {
	if cu.start.IsZero() {
		return false
	}
	return time.Since(cu.start) >= cu.threshold
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"context"
	"errors"
	"testing"
	"time"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	continuousHealthCheckTimeout = 5 * time.Second
)

// clusterUnavailabilityThreshold is the accepted duration for the cluster to temporarily not respond to requests
// (eg. during leader elections in the middle of a rolling upgrade).
func clusterUnavailabilityThreshold(b Builder) time.Duration {
	cluster := b.Elasticsearch
	if b.MutatedFrom != nil {
		cluster = b.MutatedFrom.Elasticsearch
	}
	v := version.MustParse(cluster.Spec.Version)
	if (&v).IsSameOrAfter(version.MustParse("7.2.0")) {
		// in version 7.2 and above, there is usually close to zero unavailability when a master node is killed
		// we still keep an arbitrary safety margin
		return 20 * time.Second
	}
	// in other versions (< 7.2), we commonly get about 50sec unavailability, and more on a stressed test environment
	// let's take a larger arbitrary safety margin
	return 120 * time.Second
}

func (b Builder) UpgradeTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		test.Step{
			Name: "Applying the Elasticsearch mutation should succeed",
			Test: test.Eventually(func() error {
				var curEs esv1.Elasticsearch
				if err := k.Client.Get(k8s.ExtractNamespacedName(&b.Elasticsearch), &curEs); err != nil {
					return err
				}
				curEs.Spec = b.Elasticsearch.Spec
				// may error-out with a conflict if the resource is updated concurrently
				// hence the usage of `test.Eventually`
				return k.Client.Update(&curEs)
			}),
		},
	}
}

func (b Builder) MutationTestSteps(k *test.K8sClient) test.StepList {
	var clusterIDBeforeMutation string
	var continuousHealthChecks *ContinuousHealthCheck
	var dataIntegrityCheck *DataIntegrityCheck
	mutatedFrom := b.MutatedFrom
	if mutatedFrom == nil {
		// cluster mutates to itself (same spec)
		mutatedFrom = &b
	}

	masterChangeBudgetWatcher := NewMasterChangeBudgetWatcher(b.Elasticsearch)
	changeBudgetWatcher := NewChangeBudgetWatcher(mutatedFrom.Elasticsearch.Spec, b.Elasticsearch)

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
				// Don't monitor cluster health if we're doing a rolling upgrade from a single data node cluster.
				// The cluster will become either unavailable (single node) or red (multi-nodes) when
				// that node goes down.
				return IsRollingUpgradeFromOneDataNode(b)
			},
			Test: func(t *testing.T) {
				var err error
				continuousHealthChecks, err = NewContinuousHealthCheck(b, k)
				require.NoError(t, err)
				continuousHealthChecks.Start()
			},
		},
		masterChangeBudgetWatcher.StartStep(k),
		changeBudgetWatcher.StartStep(k),
		RetrieveClusterUUIDStep(b.Elasticsearch, k, &clusterIDBeforeMutation),
	}.
		WithSteps(AnnotatePodsWithBuilderHash(*mutatedFrom, k)).
		WithSteps(b.UpgradeTestSteps(k)).
		WithSteps(b.CheckK8sTestSteps(k)).
		WithSteps(b.CheckStackTestSteps(k)).
		WithSteps(test.StepList{
			CompareClusterUUIDStep(b.Elasticsearch, k, &clusterIDBeforeMutation),
			masterChangeBudgetWatcher.StopStep(k),
			changeBudgetWatcher.StopStep(k),
			test.Step{
				Name: "Elasticsearch cluster health should not have been red during mutation process",
				Skip: func() bool {
					return IsRollingUpgradeFromOneDataNode(b)
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

func IsRollingUpgradeFromOneDataNode(b Builder) bool {
	if b.MutatedFrom == nil {
		return false
	}
	if MustNumDataNodes(b.MutatedFrom.Elasticsearch) == 1 && b.TriggersRollingUpgrade() {
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
	b               Builder
	SuccessCount    int
	FailureCount    int
	Failures        []ContinuousHealthCheckFailure
	stopChan        chan struct{}
	esClientFactory func() (esclient.Client, error)
}

// NewContinuousHealthCheck sets up a ContinuousHealthCheck struct
func NewContinuousHealthCheck(b Builder, k *test.K8sClient) (*ContinuousHealthCheck, error) {
	return &ContinuousHealthCheck{
		b:        b,
		stopChan: make(chan struct{}),
		esClientFactory: func() (esclient.Client, error) {
			return NewElasticsearchClient(b.Elasticsearch, k)
		},
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
	clusterUnavailability := clusterUnavailability{threshold: clusterUnavailabilityThreshold(hc.b)}
	go func() {
		ticker := time.NewTicker(test.DefaultRetryDelay)
		for {
			select {
			case <-hc.stopChan:
				return
			case <-ticker.C:
				// recreate the Elasticsearch client at each iteration, since we may have switched protocol from http to https during the mutation
				client, err := hc.esClientFactory()
				if err != nil {
					// treat client creation failure same as unavailable cluster
					hc.AppendErr(err)
					continue
				}
				ctx, cancel := context.WithTimeout(context.Background(), continuousHealthCheckTimeout)
				defer cancel()
				health, err := client.GetClusterHealth(ctx)
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
				if health.Status == esv1.ElasticsearchRedHealth {
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

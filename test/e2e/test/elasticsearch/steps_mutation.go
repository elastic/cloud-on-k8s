// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/generation"
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
	if (&v).GTE(version.MustParse("7.2.0")) {
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
				if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.Elasticsearch), &curEs); err != nil {
					return err
				}
				// merge annotations
				if curEs.Annotations == nil {
					curEs.Annotations = make(map[string]string)
				}
				for k, v := range b.Elasticsearch.Annotations {
					curEs.Annotations[k] = v
				}
				curEs.Spec = b.Elasticsearch.Spec
				// may error-out with a conflict if the resource is updated concurrently
				// hence the usage of `test.Eventually`
				return k.Client.Update(context.Background(), &curEs)
			}),
		},
	}
}

func (b Builder) MutationTestSteps(k *test.K8sClient) test.StepList {
	var clusterIDBeforeMutation string
	var clusterGenerationBeforeMutation, clusterObservedGenerationBeforeMutation int64
	var continuousHealthChecks *ContinuousHealthCheck
	var dataIntegrityCheck *DataIntegrityCheck
	mutatedFrom := b.MutatedFrom //nolint:ifshort
	isMutated := true
	if mutatedFrom == nil {
		// cluster mutates to itself (same spec)
		mutatedFrom = &b
		isMutated = false
	}

	var watchers []test.Watcher
	isNonHAUpgrade := IsNonHAUpgrade(b)
	if !isNonHAUpgrade {
		watchers = []test.Watcher{
			NewChangeBudgetWatcher(mutatedFrom.Elasticsearch.Spec, b.Elasticsearch),
			NewMasterChangeBudgetWatcher(b.Elasticsearch),
		}
	}

	//nolint:thelper
	steps := test.StepList{
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
				// Don't monitor cluster health if we're doing a rolling upgrade from a single data node or non-HA cluster.
				// The cluster will become either unavailable (1 or 2 node cluster due to loss of quorum) or red
				// (single data node when that node goes down).
				return isNonHAUpgrade
			},
			Test: func(t *testing.T) {
				var err error
				continuousHealthChecks, err = NewContinuousHealthCheck(b, k)
				require.NoError(t, err)
				continuousHealthChecks.Start()
			},
		},
		RetrieveClusterUUIDStep(b.Elasticsearch, k, &clusterIDBeforeMutation),
		generation.RetrieveGenerationsStep(&b.Elasticsearch, k, &clusterGenerationBeforeMutation, &clusterObservedGenerationBeforeMutation),
	}

	for _, watcher := range watchers {
		steps = steps.WithStep(watcher.StartStep(k))
	}

	//nolint:thelper
	steps = steps.WithSteps(AnnotatePodsWithBuilderHash(*mutatedFrom, k)).
		WithSteps(b.UpgradeTestSteps(k)).
		WithSteps(b.CheckK8sTestSteps(k)).
		WithSteps(b.CheckStackTestSteps(k)).
		WithSteps(test.StepList{
			CompareClusterUUIDStep(b.Elasticsearch, k, &clusterIDBeforeMutation),
			generation.CompareObjectGenerationsStep(&b.Elasticsearch, k, isMutated, clusterGenerationBeforeMutation, clusterObservedGenerationBeforeMutation),
			test.Step{
				Name: "Elasticsearch cluster health should not have been red during mutation process",
				Skip: func() bool {
					return IsNonHAUpgrade(b)
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
				Test: test.Eventually(func() error {
					return dataIntegrityCheck.Verify()
				}),
				OnFailure: printShardsAndAllocation(func() (esclient.Client, error) {
					return NewElasticsearchClient(b.Elasticsearch, k)
				}),
			},
		})

	for _, watcher := range watchers {
		steps = steps.WithStep(watcher.StopStep(k))
	}
	return steps
}

func IsNonHAUpgrade(b Builder) bool {
	if b.MutatedFrom == nil {
		return false
	}
	// a cluster of less than 3 nodes is by definition not HA will see some downtime during upgrades
	// a cluster with just one data node will also see some index level unavailability
	if (MustNumMasterNodes(b.MutatedFrom.Elasticsearch) < 3 || MustNumDataNodes(b.MutatedFrom.Elasticsearch) == 1) &&
		b.TriggersRollingUpgrade() {
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
					// according to https://github.com/kubernetes/client-go/blob/fb61a7c88cb9f599363919a34b7c54a605455ffc/rest/request.go#L959-L960,
					// client-go requests may return *errors.StatusError or *errors.UnexpectedObjectError, or http client errors.
					// It turns out catching network errors (timeout, connection refused, dns problem) is not trivial
					// (see https://stackoverflow.com/questions/22761562/portable-way-to-detect-different-kinds-of-network-error-in-golang),
					// so here we do the opposite: catch expected apiserver errors, and consider the rest are network errors.
					switch err.(type) { //nolint:errorlint
					case *k8serrors.StatusError, *k8serrors.UnexpectedObjectError:
						// explicit apiserver error, consider as healthcheck failure
						hc.AppendErr(err)
					default:
						// likely a network error, log and ignore
						fmt.Printf("error while creating the Elasticsearch client: %s", err)
					}
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

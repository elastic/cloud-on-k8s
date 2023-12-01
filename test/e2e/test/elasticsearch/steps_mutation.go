// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	k8serrors "k8s.io/apimachinery/pkg/util/errors"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/generation"
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
	switch {
	case (&v).GTE(version.MinFor(8, 3, 0)):
		// as of 8.3. we see longer unavailability. This increases the timeout until the underlying problem is better understood,
		// see: https://github.com/elastic/cloud-on-k8s/issues/5865
		return 40 * time.Second
	case (&v).GTE(version.MinFor(7, 2, 0)):
		// in version 7.2 and above, there is usually close to zero unavailability when a master node is killed
		// we still keep an arbitrary safety margin.
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
				// defensive copy as the spec struct contains nested objects like ucfg.Config that don't marshal/unmarshal
				// without losing type information making later comparisons with deepEqual fail.
				curEs.Spec = *b.Elasticsearch.Spec.DeepCopy()
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
	mutatedFrom := b.MutatedFrom
	isMutated := true
	if mutatedFrom == nil {
		// cluster mutates to itself (same spec)
		mutatedFrom = &b
		isMutated = false
	}

	watchers := []test.Watcher{
		newNodeShutdownWatcher(b.Elasticsearch),
	}

	isNonHAUpgrade := IsNonHAUpgrade(b)
	if !isNonHAUpgrade {
		watchers = append(watchers,
			NewChangeBudgetWatcher(mutatedFrom.Elasticsearch.Spec, b.Elasticsearch),
			NewMasterChangeBudgetWatcher(b.Elasticsearch),
		)
	}

	//nolint:thelper
	steps := test.StepList{
		test.Step{
			Name: "Add some data to the cluster before starting the mutation",
			Test: func(t *testing.T) {
				dataIntegrityCheck = NewDataIntegrityCheck(k, b)
				test.Eventually(func() error {
					return dataIntegrityCheck.Init()
				})(t)
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
		// avoid closure over loop iteration variable which will become the receiver of StartStep
		// leading to only one watchFn being executed
		w := watcher
		steps = steps.WithStep(w.StartStep(k))
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
					return isNonHAUpgrade
				},
				Test: func(t *testing.T) {
					continuousHealthChecks.Stop()
					if continuousHealthChecks.FailureCount > b.mutationToleratedChecksFailureCount {
						t.Errorf("ContinuousHealthChecks failures count (%d) is above the tolerance (%d): %s",
							continuousHealthChecks.FailureCount, b.mutationToleratedChecksFailureCount, continuousHealthChecks.FailuresAsString())
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
		w := watcher
		steps = steps.WithStep(w.StopStep(k))
	}
	return steps
}

// IsNonHASpec return true if the cluster specified as not highly available.
// A cluster of less than 3 nodes is by definition not HA and will see some downtime during upgrades.
// A cluster with just one data node will also see some index-level unavailability.
// We have this as a separate function in tests because in production code we base this decision on actually existing
// Pods in combination with expected Pods. Using the spec allows to control test flows before the cluster has been
// created.
func IsNonHASpec(es esv1.Elasticsearch) bool {
	return MustNumMasterNodes(es) < 3 || MustNumDataNodes(es) == 1
}

func IsNonHAUpgrade(b Builder) bool {
	if b.MutatedFrom == nil {
		return false
	}
	if IsNonHASpec(b.MutatedFrom.Elasticsearch) && b.TriggersRollingUpgrade() {
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
					fmt.Printf("error while creating the Elasticsearch client: %s\n", err)
					if !errors.As(err, &PotentialNetworkError) {
						// explicit apiserver error, consider as healthcheck failure
						hc.AppendErr(err)
					}
					continue
				}
				ctx, cancel := context.WithTimeout(context.Background(), continuousHealthCheckTimeout)
				health, err := client.GetClusterHealth(ctx)
				if err != nil {
					// Could not retrieve cluster health, can happen when the master node is killed
					// during a rolling upgrade. We allow it, unless it lasts for too long.
					clusterUnavailability.markUnavailable(err)
					if clusterUnavailability.hasExceededThreshold() {
						// cluster has been unavailable for too long
						hc.AppendErr(clusterUnavailability.Errors())
					}
					cancel()
					continue
				}
				clusterUnavailability.markAvailable()
				if health.Status == esv1.ElasticsearchRedHealth {
					hc.AppendErr(errors.New("cluster health red"))
					cancel()
					continue
				}
				hc.SuccessCount++
				cancel()
			}
		}
	}()
}

// Stop the health checks goroutine
func (hc *ContinuousHealthCheck) Stop() {
	hc.stopChan <- struct{}{}
}

// FailuresAsString returns a list of the total number of each failure as a string
func (hc *ContinuousHealthCheck) FailuresAsString() string {
	if len(hc.Failures) == 0 {
		return "0 failure"
	}
	errCountMap := map[string]int{}
	for _, f := range hc.Failures {
		errCountMap[f.err.Error()]++
	}
	strList := []string{}
	for err, total := range errCountMap {
		strList = append(strList, fmt.Sprintf("%d x [%s]", total, err))
	}
	return strings.Join(strList, "\n")
}

type clusterUnavailability struct {
	start     time.Time
	threshold time.Duration
	errors    []error
}

func (cu *clusterUnavailability) markUnavailable(err error) {
	if cu.start.IsZero() {
		cu.start = time.Now()
	}
	if err != nil {
		cu.errors = append(cu.errors, err)
	}
}

func (cu *clusterUnavailability) markAvailable() {
	cu.start = time.Time{}
	cu.errors = nil
}

func (cu *clusterUnavailability) hasExceededThreshold() bool {
	if cu.start.IsZero() {
		return false
	}
	return time.Since(cu.start) >= cu.threshold
}

func (cu *clusterUnavailability) Errors() error {
	return k8serrors.NewAggregate(cu.errors)
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"context"
	"errors"
	"testing"
	"time"

	estype "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const continuousHealthCheckTimeout = 25 * time.Second

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

func (b Builder) MutationTestSteps(k *test.K8sClient, options test.MutationOptions) test.StepList {
	var clusterIDBeforeMutation string
	var continuousHealthChecks *ContinuousHealthCheck
	var dataIntegrityCheck *DataIntegrityCheck

	return test.StepList{
		test.Step{
			Name: "Add some data to the cluster before starting the mutation",
			Test: func(t *testing.T) {
				var err error
				dataIntegrityCheck, err = NewDataIntegrityCheck(b.Elasticsearch, k, options.IncludesRollingUpgrade)
				require.NoError(t, err)
				require.NoError(t, dataIntegrityCheck.Init())
			},
		},
		test.Step{
			Name: "Start querying Elasticsearch cluster health while mutation is going on",
			Test: func(t *testing.T) {
				var err error
				continuousHealthChecks, err = NewContinuousHealthCheck(b.Elasticsearch, k)
				require.NoError(t, err)
				continuousHealthChecks.Start()
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
				Name: "Elasticsearch cluster health should not have been red during mutation process",
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
				Test: func(t *testing.T) {
					require.NoError(t, dataIntegrityCheck.Verify())
				},
			},
		})
}

// ContinuousHealthCheckFailure represents an health check failure
type ContinuousHealthCheckFailure struct {
	err       error
	timestamp time.Time
}

// ContinuousHealthCheck continuously runs health checks against Elasticsearch
// during the whole mutation process
type ContinuousHealthCheck struct {
	SuccessCount int
	FailureCount int
	Failures     []ContinuousHealthCheckFailure
	stopChan     chan struct{}
	esClient     esclient.Client
}

// NewContinuousHealthCheck sets up a ContinuousHealthCheck struct
func NewContinuousHealthCheck(es estype.Elasticsearch, k *test.K8sClient) (*ContinuousHealthCheck, error) {
	esClient, err := NewElasticsearchClient(es, k)
	if err != nil {
		return nil, err
	}
	return &ContinuousHealthCheck{
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
					// TODO: Temporarily account only red clusters, see https://github.com/elastic/cloud-on-k8s/issues/614
					// hc.AppendErr(err)
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
func (hc *ContinuousHealthCheck) Stop() {
	hc.stopChan <- struct{}{}
}

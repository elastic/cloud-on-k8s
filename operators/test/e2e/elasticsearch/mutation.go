// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"context"
	"errors"
	"testing"
	"time"

	estype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	esclient "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const continousHealthCheckTimeout = 25 * time.Second

func (b Builder) MutationTestSteps(k *helpers.K8sHelper) helpers.TestStepList {

	var clusterIDBeforeMutation string

	var continuousHealthChecks *ContinousHealthCheck

	return helpers.TestStepList{
		helpers.TestStep{
			Name: "Start querying Elasticsearch cluster health while mutation is going on",
			Test: func(t *testing.T) {
				var err error
				continuousHealthChecks, err = NewContinousHealthCheck(b.Elasticsearch, k)
				require.NoError(t, err)
				continuousHealthChecks.Start()
			},
		},
		RetrieveClusterUUIDStep(b.Elasticsearch, k, &clusterIDBeforeMutation),
		helpers.TestStep{
			Name: "Applying the Elasticsearch mutation should succeed",
			Test: func(t *testing.T) {
				var curEs estype.Elasticsearch
				require.NoError(t, k.Client.Get(k8s.ExtractNamespacedName(&b.Elasticsearch), &curEs))
				curEs.Spec = b.Elasticsearch.Spec
				require.NoError(t, k.Client.Update(&curEs))
			},
		},
	}.
		WithSteps(b.CheckStackSteps(k)).
		WithSteps(helpers.TestStepList{
			CompareClusterUUIDStep(b.Elasticsearch, k, &clusterIDBeforeMutation),
			helpers.TestStep{
				Name: "Elasticsearch cluster health should not have been red during mutation process",
				Test: func(t *testing.T) {
					continuousHealthChecks.Stop()
					assert.Equal(t, 0, continuousHealthChecks.FailureCount)
					for _, f := range continuousHealthChecks.Failures {
						t.Errorf("Elasticsearch cluster health check failure at %s: %s", f.timestamp, f.err.Error())
					}
				},
			},
		})
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

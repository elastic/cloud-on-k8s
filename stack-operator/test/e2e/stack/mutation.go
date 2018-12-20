package stack

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/deployments/v1alpha1"
	estype "github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	esclient "github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
	"github.com/elastic/stack-operators/stack-operator/test/e2e/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	continousHealthCheckInterval = 5 * time.Second
	continousHealthCheckTimeout  = 10 * time.Second
)

// MutationTestSteps tests topology changes on the given stack
// we expect the stack to be already created and running.
// If the stack to mutate to is the same as the original stack,
// then all tests should still pass.
func MutationTestSteps(stack v1alpha1.Stack, k *helpers.K8sHelper) []helpers.TestStep {

	var clusterIDBeforeMutation string

	var continuousHealthChecks *ContinousHealthCheck

	return helpers.TestStepList{}.
		WithSteps(
			helpers.TestStep{
				Name: "Start querying ES cluster health while mutation is going on",
				Test: func(t *testing.T) {
					var err error
					continuousHealthChecks, err = NewContinousHealthCheck(stack, k)
					require.NoError(t, err)
					continuousHealthChecks.Start()
				},
			},
			helpers.TestStep{
				Name: "Retrieve cluster ID before mutation for comparison purpose",
				Test: helpers.Eventually(func() error {
					var s v1alpha1.Stack
					err := k.Client.Get(helpers.DefaultCtx, GetNamespacedName(stack), &s)
					if err != nil {
						return err
					}
					clusterIDBeforeMutation = s.Status.Elasticsearch.ClusterUUID
					if clusterIDBeforeMutation == "" {
						return fmt.Errorf("Empty ClusterUUID")
					}
					return nil
				}),
			},
			helpers.TestStep{
				Name: "Applying the mutation should succeed",
				Test: func(t *testing.T) {
					// get stack so we have a versioned k8s resource we can update
					var stackRes v1alpha1.Stack
					err := k.Client.Get(helpers.DefaultCtx, GetNamespacedName(stack), &stackRes)
					require.NoError(t, err)
					// update with new stack spec
					stackRes.Spec = stack.Spec
					err = k.Client.Update(helpers.DefaultCtx, &stackRes)
					require.NoError(t, err)
				},
			}).
		WithSteps(CheckStackSteps(stack, k)...).
		WithSteps(
			helpers.TestStep{
				Name: "Cluster UUID should be preserved after mutation is done",
				Test: func(t *testing.T) {
					var s v1alpha1.Stack
					err := k.Client.Get(helpers.DefaultCtx, GetNamespacedName(stack), &s)
					require.NoError(t, err)
					clusterIDAfterMutation := s.Status.Elasticsearch.ClusterUUID
					require.NotEmpty(t, clusterIDBeforeMutation)
					require.Equal(t, clusterIDBeforeMutation, clusterIDAfterMutation)
				},
			},
			helpers.TestStep{
				Name: "Cluster health should not have been red during mutation process",
				Test: func(t *testing.T) {
					continuousHealthChecks.Stop()
					assert.NotZero(t, continuousHealthChecks.SuccessCount)
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
	esClient     *esclient.Client
}

// NewContinousHealthCheck sets up a ContinousHealthCheck struct
func NewContinousHealthCheck(stack v1alpha1.Stack, k *helpers.K8sHelper) (*ContinousHealthCheck, error) {
	esClient, err := helpers.NewElasticsearchClient(stack, k)
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
		ticker := time.NewTicker(continousHealthCheckInterval)
		for {
			select {
			case <-hc.stopChan:
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), continousHealthCheckTimeout)
				defer cancel()
				health, err := hc.esClient.GetClusterHealth(ctx)
				if err != nil {
					hc.AppendErr(err)
					continue
				}
				if estype.ElasticsearchHealth(health.Status) == estype.ElasticsearchRedHealth {
					hc.AppendErr(errors.New("Cluster health red"))
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

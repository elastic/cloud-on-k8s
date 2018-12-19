package stack

import (
	"context"
	"fmt"
	"testing"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/deployments/v1alpha1"
	estype "github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
	"github.com/elastic/stack-operators/stack-operator/test/e2e/helpers"
	"github.com/stretchr/testify/assert"
)

type esClusterChecks struct {
	client *client.Client
}

// ESClusterChecks returns all test steps to verify the given stack's Elasticsearch
// cluster is running as expected
func ESClusterChecks(stack v1alpha1.Stack, k *helpers.K8sHelper) helpers.TestStepList {
	e := esClusterChecks{}
	return helpers.TestStepList{
		e.BuildESClient(stack, k),
		e.CheckESReachable(),
		e.CheckESHealthGreen(),
	}
}

func (e *esClusterChecks) BuildESClient(stack v1alpha1.Stack, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "Every secret should be set so that we can build an ES client",
		Test: func(t *testing.T) {
			esClient, err := helpers.NewElasticsearchClient(stack, k)
			assert.NoError(t, err)
			e.client = esClient
		},
	}
}

func (e *esClusterChecks) CheckESReachable() helpers.TestStep {
	return helpers.TestStep{
		Name: "Elasticsearch endpoint should eventually be reachable",
		Test: helpers.Eventually(func() error {
			if _, err := e.client.GetClusterHealth(context.TODO()); err != nil {
				return err
			}
			return nil
		}),
	}
}

func (e *esClusterChecks) CheckESHealthGreen() helpers.TestStep {
	return helpers.TestStep{
		Name: "Elasticsearch endpoint should eventually be reachable",
		Test: helpers.Eventually(func() error {
			health, err := e.client.GetClusterHealth(context.TODO())
			if err != nil {
				return err
			}
			actualHealth := estype.ElasticsearchHealth(health.Status)
			expectedHealth := estype.ElasticsearchGreenHealth
			if actualHealth != expectedHealth {
				return fmt.Errorf("Cluster health is not green, but %s", actualHealth)
			}
			return nil
		}),
	}
}

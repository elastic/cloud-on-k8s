// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apm

import (
	"context"
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"

	apmtype "github.com/elastic/k8s-operators/operators/pkg/apis/apm/v1alpha1"
	estype "github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/test/e2e/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type apmClusterChecks struct {
	apmClient *helpers.ApmClient
	esClient  client.Client
}

// ApmServerChecks returns all test steps to verify the given Apm Server is running as expected
func ApmServerChecks(as apmtype.ApmServer, es estype.Elasticsearch, k *helpers.K8sHelper) helpers.TestStepList {
	a := apmClusterChecks{}
	return helpers.TestStepList{
		a.BuildApmServerClient(as, es, k),
		a.CheckApmServerReachable(),
		a.CheckApmServerVersion(as),
		a.CheckEventsAPI(),
	}
}

func (c *apmClusterChecks) BuildApmServerClient(
	as apmtype.ApmServer,
	es estype.Elasticsearch,
	k *helpers.K8sHelper,
) helpers.TestStep {
	return helpers.TestStep{
		Name: "Every secret should be set so that we can build an APM client",
		Test: func(t *testing.T) {
			helpers.Eventually(func() error {
				// fetch the latest ApmServer resource from the API because we need to get resources that are provided
				// by the controller as part of the Status section
				var updatedApmServer apmtype.ApmServer
				if err := k.Client.Get(k8s.ExtractNamespacedName(&as), &updatedApmServer); err != nil {
					return err
				}

				apmClient, err := helpers.NewApmServerClient(updatedApmServer, k)
				if err != nil {
					return err
				}
				c.apmClient = apmClient
				return nil
			})(t)

			esClient, err := helpers.NewElasticsearchClient(es, k)
			assert.NoError(t, err)
			c.esClient = esClient
		},
	}
}

func (c *apmClusterChecks) CheckApmServerReachable() helpers.TestStep {
	return helpers.TestStep{
		Name: "ApmServer endpoint should eventually be reachable",
		Test: helpers.Eventually(func() error {
			if _, err := c.apmClient.ServerInfo(context.TODO()); err != nil {
				return err
			}
			return nil
		}),
	}
}

func (c *apmClusterChecks) CheckApmServerVersion(as apmtype.ApmServer) helpers.TestStep {
	return helpers.TestStep{
		Name: "ApmServer version should be the expected one",
		Test: func(t *testing.T) {
			info, err := c.apmClient.ServerInfo(context.TODO())
			require.NoError(t, err)

			require.Equal(t, as.Spec.Version, info.Version)
		},
	}
}

func (c *apmClusterChecks) CheckEventsAPI() helpers.TestStep {
	sampleBody := `{"metadata": { "service": {"name": "1234_service-12a3", "language": {"name": "ecmascript"}, "agent": {"version": "3.14.0", "name": "elastic-node"}}}}
{ "error": {"id": "abcdef0123456789", "timestamp": 1533827045999000,"log": {"level": "custom log level","message": "Cannot read property 'baz' of undefined"}}}
{ "metricset": { "samples": { "go.memstats.heap.sys.bytes": { "value": 61235 } }, "timestamp": 1496170422281000 }}`

	return helpers.TestStep{
		Name: "Events should be accepted",
		Test: func(t *testing.T) {
			eventsErrorResponse, err := c.apmClient.IntakeV2Events(context.TODO(), []byte(sampleBody))
			require.NoError(t, err)

			// in the happy case, we get no error response
			assert.Nil(t, eventsErrorResponse)
			if eventsErrorResponse != nil {
				// provide more details:
				assert.Equal(t, eventsErrorResponse.Accepted, 2)
				assert.Len(t, eventsErrorResponse.Errors, 0)
			}

			// TODO: verify that the events eventually show up in Elasticsearch
		},
	}
}

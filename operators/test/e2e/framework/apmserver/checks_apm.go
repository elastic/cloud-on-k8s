// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserver

import (
	"context"
	"testing"

	apmtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/apm/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/framework"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type apmClusterChecks struct {
	apmClient *ApmClient
}

func (b Builder) CheckStackTestSteps(k *framework.K8sClient) framework.TestStepList {
	a := apmClusterChecks{}
	return framework.TestStepList{
		a.BuildApmServerClient(b.ApmServer, k),
		a.CheckApmServerReachable(),
		a.CheckApmServerVersion(b.ApmServer),
		a.CheckEventsAPI(),
	}
}

func (c *apmClusterChecks) BuildApmServerClient(apm apmtype.ApmServer, k *framework.K8sClient,
) framework.TestStep {
	return framework.TestStep{
		Name: "Every secret should be set so that we can build an APM client",
		Test: func(t *testing.T) {
			framework.Eventually(func() error {
				// fetch the latest APM Server resource from the API because we need to get resources that are provided
				// by the controller apm part of the status section
				var updatedApmServer apmtype.ApmServer
				if err := k.Client.Get(k8s.ExtractNamespacedName(&apm), &updatedApmServer); err != nil {
					return err
				}

				apmClient, err := NewApmServerClient(updatedApmServer, k)
				if err != nil {
					return err
				}
				c.apmClient = apmClient
				return nil
			})(t)
		},
	}
}

func (c *apmClusterChecks) CheckApmServerReachable() framework.TestStep {
	return framework.TestStep{
		Name: "ApmServer endpoint should eventually be reachable",
		Test: framework.Eventually(func() error {
			ctx, cancel := context.WithTimeout(context.Background(), DefaultReqTimeout)
			defer cancel()
			if _, err := c.apmClient.ServerInfo(ctx); err != nil {
				return err
			}
			return nil
		}),
	}
}

func (c *apmClusterChecks) CheckApmServerVersion(apm apmtype.ApmServer) framework.TestStep {
	return framework.TestStep{
		Name: "ApmServer version should be the expected one",
		Test: func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), DefaultReqTimeout)
			defer cancel()
			info, err := c.apmClient.ServerInfo(ctx)
			require.NoError(t, err)

			require.Equal(t, apm.Spec.Version, info.Version)
		},
	}
}

func (c *apmClusterChecks) CheckEventsAPI() framework.TestStep {
	sampleBody := `{"metadata": { "service": {"name": "1234_service-12a3", "language": {"name": "ecmascript"}, "agent": {"version": "3.14.0", "name": "elastic-node"}}}}
{ "error": {"id": "abcdef0123456789", "timestamp": 1533827045999000,"log": {"level": "custom log level","message": "Cannot read property 'baz' of undefined"}}}
{ "metricset": { "samples": { "go.memstats.heap.sys.bytes": { "value": 61235 } }, "timestamp": 1496170422281000 }}`

	return framework.TestStep{
		Name: "Events should be accepted",
		Test: func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), DefaultReqTimeout)
			defer cancel()
			eventsErrorResponse, err := c.apmClient.IntakeV2Events(ctx, []byte(sampleBody))
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

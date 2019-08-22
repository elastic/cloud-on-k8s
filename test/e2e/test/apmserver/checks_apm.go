// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

	apmtype "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
)

type apmClusterChecks struct {
	apmClient *ApmClient
	esClient  client.Client
}

func (b Builder) CheckStackTestSteps(k *test.K8sClient) test.StepList {
	a := apmClusterChecks{}
	return test.StepList{
		a.BuildApmServerClient(b.ApmServer, k),
		a.CheckApmServerReachable(),
		a.CheckApmServerVersion(b.ApmServer),
		a.CheckEventsAPI(),
		a.CheckEventsInElasticsearch(b.ApmServer, k),
	}
}

func (c *apmClusterChecks) BuildApmServerClient(apm apmtype.ApmServer, k *test.K8sClient,
) test.Step {
	return test.Step{
		Name: "Every secret should be set so that we can build an APM client",
		Test: func(t *testing.T) {
			test.Eventually(func() error {
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

				// Get the associated Elasticsearch
				if !apm.Spec.ElasticsearchRef.IsDefined() { // No associated ES, do not try to create a client
					return nil
				}

				// We assume here that the Elasticsearch object has been created before the APM Server.
				var es v1alpha1.Elasticsearch
				namespace := apm.Spec.ElasticsearchRef.Namespace
				if len(namespace) == 0 {
					namespace = apm.Namespace
				}
				if err := k.Client.Get(types.NamespacedName{
					Namespace: namespace,
					Name:      apm.Spec.ElasticsearchRef.Name,
				}, &es); err != nil {
					return err
				}
				// Build the Elasticsearch client
				c.esClient, err = elasticsearch.NewElasticsearchClient(es, k)
				return err
			})(t)
		},
	}
}

func (c *apmClusterChecks) CheckApmServerReachable() test.Step {
	return test.Step{
		Name: "ApmServer endpoint should eventually be reachable",
		Test: test.Eventually(func() error {
			ctx, cancel := context.WithTimeout(context.Background(), DefaultReqTimeout)
			defer cancel()
			if _, err := c.apmClient.ServerInfo(ctx); err != nil {
				return err
			}
			return nil
		}),
	}
}

func (c *apmClusterChecks) CheckApmServerVersion(apm apmtype.ApmServer) test.Step {
	return test.Step{
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

func (c *apmClusterChecks) CheckEventsAPI() test.Step {
	sampleBody := `{"metadata": { "service": {"name": "1234_service-12a3", "language": {"name": "ecmascript"}, "agent": {"version": "3.14.0", "name": "elastic-node"}}}}
{ "error": {"id": "abcdef0123456789", "timestamp": 1533827045999000,"log": {"level": "custom log level","message": "Cannot read property 'baz' of undefined"}}}
{ "metricset": { "samples": { "go.memstats.heap.sys.bytes": { "value": 61235 } }, "timestamp": 1496170422281000 }}`

	return test.Step{
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
		},
	}
}

// CountResult maps the result of a /index/_count request.
type CountResult struct {
	Count  int `json:"count"`
	Shards struct {
		Total      int `json:"total"`
		Successful int `json:"successful"`
		Skipped    int `json:"skipped"`
		Failed     int `json:"failed"`
	} `json:"_shards"`
}

// CheckEventsInElasticsearch checks that the events sent in the previous step have been stored.
// We only count document to not rely on the internal schema of the APM Server.
func (c *apmClusterChecks) CheckEventsInElasticsearch(apm apmtype.ApmServer, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "Events should eventually show up in Elasticsearch",
		Test: test.Eventually(func() error {
			// Fetch the last version of the APM Server
			var updatedApmServer apmtype.ApmServer
			if err := k.Client.Get(k8s.ExtractNamespacedName(&apm), &updatedApmServer); err != nil {
				return err
			}

			if !updatedApmServer.Spec.ElasticsearchRef.IsDefined() {
				// No ES is referenced, do not try to check data
				return nil
			}

			// Check that the metric has been stored
			err := assertCountIndexEqual(
				c.esClient,
				fmt.Sprintf("apm-%s-metric-2017.05.30", updatedApmServer.Spec.Version),
				1,
			)
			if err != nil {
				return err
			}

			// Check that the error has been stored
			err = assertCountIndexEqual(
				c.esClient,
				fmt.Sprintf("apm-%s-error-2018.08.09", updatedApmServer.Spec.Version),
				1,
			)
			if err != nil {
				return err
			}

			return nil
		}),
	}
}

// assertCountIndexEqual asserts that the number of document in an index is the expected one, it raises an error otherwise.
func assertCountIndexEqual(esClient client.Client, index string, expected int) error {
	metricCount, err := countIndex(esClient, index)
	if err != nil {
		return err
	}
	if metricCount != expected {
		return fmt.Errorf("%d document expected in index %s, got %d instead", expected, index, metricCount)
	}
	return nil
}

// countIndex counts the number of document in an index.
func countIndex(esClient client.Client, indexName string) (int, error) {
	r, err := http.NewRequest(
		http.MethodGet, fmt.Sprintf("/%s/_count", indexName),
		nil,
	)
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), DefaultReqTimeout)
	defer cancel()
	response, err := esClient.Request(ctx, r)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close() // nolint
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return 0, err
	}

	// Unmarshal the response
	var countResult CountResult
	err = json.Unmarshal(body, &countResult)
	if err != nil {
		return 0, err
	}
	return countResult.Count, nil
}

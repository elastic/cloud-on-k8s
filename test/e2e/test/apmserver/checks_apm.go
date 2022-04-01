// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package apmserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/kibana"
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
		a.CheckIndexCreation(b.ApmServer, k),
		a.CheckEventsAPI(),
		a.CheckEventsInElasticsearch(b.ApmServer, k),
		a.CheckRUMEventsAPI(b.RUMEnabled()),
	}.WithSteps(a.CheckAgentConfiguration(b.ApmServer, k))
}

//nolint:thelper
func (c *apmClusterChecks) BuildApmServerClient(apm apmv1.ApmServer, k *test.K8sClient,
) test.Step {
	return test.Step{
		Name: "Every secret should be set so that we can build an APM client",
		Test: func(t *testing.T) {
			test.Eventually(func() error {
				// fetch the latest APM Server resource from the API because we need to get resources that are provided
				// by the controller apm part of the status section
				var updatedApmServer apmv1.ApmServer
				if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&apm), &updatedApmServer); err != nil {
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
				var es esv1.Elasticsearch
				namespace := apm.Spec.ElasticsearchRef.Namespace
				if len(namespace) == 0 {
					namespace = apm.Namespace
				}
				if err := k.Client.Get(context.Background(), types.NamespacedName{
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

//nolint:thelper
func (c *apmClusterChecks) CheckApmServerVersion(apm apmv1.ApmServer) test.Step {
	return test.Step{
		Name: "ApmServer version should be the expected one",
		Test: func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), DefaultReqTimeout)
			defer cancel()
			info, err := c.apmClient.ServerInfo(ctx)
			require.NoError(t, err)

			require.Equal(t, apm.EffectiveVersion(), info.Version)
		},
	}
}

// CheckIndexCreation ensure that, prior to attempting to ingest events, the APM Server user
// has the necessary permissions to either create indexes (ES < 8.x), or index documents into
// non-existing indexes (ES >= 8.x). This fixes a transient issue that happens when upgrading
// Elasticsearch between major versions where it takes a bit of time to transition between
// file-based user roles, and permissions errors were being returned by Elasticsearch.
func (c *apmClusterChecks) CheckIndexCreation(apm apmv1.ApmServer, k *test.K8sClient) test.Step {
	// default to < 8.x Elasticsearch APM Server index names
	indexName := "apm-testindex-" + rand.String(4)

	return test.Step{
		Name: "ES Index should eventually be able to be created by APM Server user",
		Test: test.Eventually(func() error {
			if !apm.Spec.ElasticsearchRef.IsDefined() {
				return nil
			}

			ctx, cancel := context.WithTimeout(context.Background(), DefaultReqTimeout)
			defer cancel()

			managedNamespace := test.Ctx().ManagedNamespace(0)

			sec := corev1.Secret{}
			if err := k.Client.Get(ctx, types.NamespacedName{Name: apm.Name + "-apm-user", Namespace: managedNamespace}, &sec); err != nil {
				return errors.Wrap(err, "while getting apm user secret")
			}

			usernameKey := fmt.Sprintf("%s-%s-apm-user", managedNamespace, apm.Name)
			b, ok := sec.Data[usernameKey]
			if !ok {
				return fmt.Errorf("secret data did not contain key %s", usernameKey)
			}
			password := string(b)

			es := esv1.Elasticsearch{}
			if err := k.Client.Get(ctx, types.NamespacedName{Name: apm.Spec.ElasticsearchRef.Name, Namespace: apm.Spec.ElasticsearchRef.Namespace}, &es); err != nil {
				return errors.Wrap(err, "while getting associated Elasticsearch cluster")
			}

			esClient, err := elasticsearch.NewElasticsearchClientWithUser(es, k, client.BasicAuth{
				Name:     fmt.Sprintf("%s-%s-apm-user", managedNamespace, apm.Name),
				Password: password,
			})
			if err != nil {
				return err
			}

			r, err := http.NewRequestWithContext(
				ctx,
				http.MethodPut,
				fmt.Sprintf("/%s", indexName),
				nil,
			)
			if err != nil {
				return errors.Wrap(err, "while creating new http request")
			}

			// If the ES version is >= 8.x, then the index name must change to a datastream,
			// and we only have permissions to auto-create indexes on index document requests,
			// not explicitly create index requests.
			if version.MustParse(es.Spec.Version).GE(version.MinFor(8, 0, 0)) {
				indexName = "metrics-apm.testindex-" + rand.String(4)

				r, err = http.NewRequestWithContext(
					ctx,
					http.MethodPost,
					fmt.Sprintf("/%s/_doc/", indexName),
					strings.NewReader(`{"@timestamp": "2022-03-22T00:00:00.000Z","message": "test_message"}`),
				)
				if err != nil {
					return errors.Wrap(err, "while creating new http request")
				}
			}

			res, err := esClient.Request(ctx, r)
			if err != nil {
				return errors.Wrap(err, "while executing http request")
			}

			defer res.Body.Close()
			// we should receieve either a 200 (index creation), or 201 (index doc request)
			// from Elasticsearch
			if res.StatusCode > 201 {
				return fmt.Errorf("expected http 200/201 response code when creating index, got %d", res.StatusCode)
			}
			return nil
		}),
	}
}

//nolint:thelper
func (c *apmClusterChecks) CheckEventsAPI() test.Step {
	sampleBody := `{"metadata": { "service": {"name": "1234_service-12a3", "language": {"name": "ecmascript"}, "agent": {"version": "3.14.0", "name": "elastic-node"}}}}
{ "error": {"id": "abcdef0123456789", "timestamp": 1533827045999000,"log": {"level": "custom log level","message": "Cannot read property 'baz' of undefined"}}}
{ "metricset": { "samples": { "go.memstats.heap.sys.bytes": { "value": 61235 } }, "timestamp": 1496170422281000 }}`

	return test.Step{
		Name: "Events should be accepted",
		Test: func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), DefaultReqTimeout)
			defer cancel()
			eventsErrorResponse, err := c.apmClient.IntakeV2Events(ctx, false, []byte(sampleBody))
			require.NoError(t, err)

			// in the happy case, we get no error response
			assert.Nil(t, eventsErrorResponse)
			if eventsErrorResponse != nil {
				// provide more details:
				assert.Equal(t, 2, eventsErrorResponse.Accepted)
				assert.Len(t, eventsErrorResponse.Errors, 0)
			}
		},
	}
}

func (c *apmClusterChecks) CheckRUMEventsAPI(rumEnabled bool) test.Step {
	sampleBody := `{"metadata":{"service":{"name":"apm-agent-js","version":"1.0.0","agent":{"name":"rum-js","version":"0.0.0"}}}}
{"transaction":{"id":"611f4fa950f04631","type":"page-load","duration":643,"context":{"page":{"referer":"http://localhost:8000/test/e2e/","url":"http://localhost:8000/test/e2e/general-usecase/"}},"trace_id":"611f4fa950f04631aaaaaaaaaaaaaaaa","span_count":{"started":1}}}`

	should := "forbidden"
	assertError := assert.NotNil
	if rumEnabled {
		should = "accepted"
		assertError = assert.Nil
	}
	//nolint:thelper
	return test.Step{
		Name: "Events should be " + should,
		Test: func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), DefaultReqTimeout)
			defer cancel()
			eventsErrorResponse, err := c.apmClient.IntakeV2Events(ctx, true, []byte(sampleBody))
			require.NoError(t, err)

			assertError(t, eventsErrorResponse)
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
func (c *apmClusterChecks) CheckEventsInElasticsearch(apm apmv1.ApmServer, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "Events should eventually show up in Elasticsearch",
		Test: test.Eventually(func() error {
			// Fetch the last version of the APM Server
			var updatedApmServer apmv1.ApmServer
			if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&apm), &updatedApmServer); err != nil {
				return err
			}

			if !updatedApmServer.Spec.ElasticsearchRef.IsDefined() {
				// No ES is referenced, do not try to check data
				return nil
			}

			v, err := version.Parse(updatedApmServer.Spec.Version)
			if err != nil {
				return err
			}

			// Check that the metric and error have been stored
			// default to indices names from 6.x
			metricIndex := fmt.Sprintf("apm-%s-2017.05.30", updatedApmServer.EffectiveVersion())
			errorIndex := fmt.Sprintf("apm-%s-2018.08.09", updatedApmServer.EffectiveVersion())
			switch v.Major {
			case 7:
				metricIndex = fmt.Sprintf("apm-%s-metric-2017.05.30", updatedApmServer.EffectiveVersion())
				errorIndex = fmt.Sprintf("apm-%s-error-2018.08.09", updatedApmServer.EffectiveVersion())
			case 8:
				// these are datastreams and not indices, but can be searched/counted in the same way
				metricIndex = "metrics-apm.app.1234_service_12a3-default"
				errorIndex = "logs-apm.error-default"
			}

			if err := assertCountIndexEqual(c.esClient, metricIndex, 1); err != nil {
				return err
			}

			return assertCountIndexEqual(c.esClient, errorIndex, 1)
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
	r, err := http.NewRequest( //nolint:noctx
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
	defer response.Body.Close()
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

const sampleDefaultAgentConfiguration = `{"service":{},"settings":{"transaction_sample_rate":"1","capture_body":"errors","transaction_max_spans":"99"}}`

// CheckAgentConfiguration creates an agent configuration through Kibana and then check that the APM Server is able to retrieve it.
func (c *apmClusterChecks) CheckAgentConfiguration(apm apmv1.ApmServer, k *test.K8sClient) test.StepList {
	apmVersion := version.MustParse(apm.Spec.Version)

	if !apm.Spec.KibanaRef.IsDefined() {
		return []test.Step{}
	}

	return []test.Step{
		{
			Name: "Create the default Agent Configuration in Kibana",
			Test: test.Eventually(func() error {
				kb := kbv1.Kibana{}
				if err := k.Client.Get(context.Background(), apm.Spec.KibanaRef.WithDefaultNamespace(apm.Namespace).NamespacedName(), &kb); err != nil {
					return err
				}

				password, err := k.GetElasticPassword(apm.Spec.ElasticsearchRef.WithDefaultNamespace(apm.Namespace).NamespacedName())
				if err != nil {
					return err
				}

				uri := "/api/apm/settings/agent-configuration"

				// URI is slightly different before 7.7.0, we need to add "/new" at the end
				if !apmVersion.GTE(version.MustParse("7.7.0")) {
					uri += "/new"
				}
				_, err = kibana.DoRequest(k, kb, password, "PUT", uri, []byte(sampleDefaultAgentConfiguration))
				return err
			}),
		},
		{
			Name: "Read back the agent default configuration from the APM Server",
			Test: test.Eventually(func() error {
				ctx, cancel := context.WithTimeout(context.Background(), DefaultReqTimeout)
				defer cancel()

				agentConfig, err := c.apmClient.AgentsDefaultConfig(ctx)
				if err != nil {
					return err
				}

				expectedAgentConfiguration := AgentConfig{
					CaptureBody:           "errors",
					TransactionMaxSpans:   "99",
					TransactionSampleRate: "1",
				}
				if !reflect.DeepEqual(expectedAgentConfiguration, agentConfig) {
					return fmt.Errorf("expected agent configuration %+v, got %+v", expectedAgentConfiguration, agentConfig)
				}
				return nil
			}),
		},
	}
}

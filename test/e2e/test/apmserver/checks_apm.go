// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package apmserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"

	apmv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/apm/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	commonhttp "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/http"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/retry"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/kibana"
)

const sampleEventBody = `{"metadata": { "service": {"name": "1234_service-12a3", "language": {"name": "ecmascript"}, "agent": {"version": "3.14.0", "name": "elastic-node"}}}}
{ "error": {"id": "abcdef0123456789", "timestamp": 1533827045999000,"log": {"level": "custom log level","message": "Cannot read property 'baz' of undefined"}}}
{ "metricset": { "samples": { "go.memstats.heap.sys.bytes": { "value": 61235 } }, "timestamp": 1496170422281000 }}`

type apmClusterChecks struct {
	apmClient        *ApmClient
	esClient         client.Client
	metricIndexCount int
	errorIndexCount  int
}

func (b Builder) CheckStackTestSteps(k *test.K8sClient) test.StepList {
	a := apmClusterChecks{}
	return test.StepList{
		a.BuildApmServerClient(b.ApmServer, k),
		a.CheckApmServerReachable(),
		a.CheckApmServerVersion(b.ApmServer),
		a.CheckAPMSecretTokenConfiguration(b.ApmServer, k),
		a.CheckAPMEventCanBeIndexedInElasticsearch(b.ApmServer, k),
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

// CheckAPMEventCanBeIndexedInElasticsearch ensures that any event that is sent to APM Server
// eventually ends up within an Elasticsearch index.  The index name varies between versions.
// APM Server version < 8.x creates an index, and writes data to a named index.  APM Server
// version >= 8.x writes documents to a datastream, and an index is auto-created.
// This test step has to be eventual, as a transient issue happens when upgrading
// Elasticsearch between major versions where it takes a bit of time to transition between
// file-based user roles, and permissions errors are returned from Elasticsearch.
func (c *apmClusterChecks) CheckAPMEventCanBeIndexedInElasticsearch(apm apmv1.ApmServer, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "ApmServer should accept event and write data to Elasticsearch",
		Test: test.Eventually(func() error {
			// All APM Server tests do not have an Elasticsearch reference.
			if !apm.Spec.ElasticsearchRef.IsDefined() {
				return nil
			}
			if err := c.checkEventsAPI(apm); err != nil {
				return err
			}
			return retry.UntilSuccess(func() error {
				return c.checkEventsInElasticsearch(apm, k)
			}, 30*time.Second, 2*time.Second)
		}),
	}
}

func (c *apmClusterChecks) CheckAPMSecretTokenConfiguration(apm apmv1.ApmServer, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "APMServer should reject events with incorrect token setup",
		Test: test.Eventually(func() error {
			// All APM Server tests do not have an Elasticsearch reference.
			if !apm.Spec.ElasticsearchRef.IsDefined() {
				return nil
			}

			// as above for the functioning client: fetch the latest APM Server resource from the API because we need to
			// get resources that are provided by the controller apm part of the status section
			var updatedApmServer apmv1.ApmServer
			if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&apm), &updatedApmServer); err != nil {
				return err
			}
			client, err := NewAPMServerClientWithSecretToken(updatedApmServer, k, "not-a-valid-token")
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(context.Background(), DefaultReqTimeout)
			defer cancel()
			_, err = client.IntakeV2Events(ctx, false, []byte(sampleEventBody))
			if !commonhttp.IsUnauthorized(err) {
				return fmt.Errorf("expected error 401 but was %w", err)
			}
			return nil
		}),
	}
}

func (c *apmClusterChecks) checkEventsAPI(apm apmv1.ApmServer) error {
	// before sending event, get the document count in the metric, and error index
	// and save, as it is used to calculate how many docs should be in the index after
	// the event is sent through APM Server.
	metricIndex, errorIndex, err := getIndexNames(apm)
	if err != nil {
		return err
	}

	var count int
	count, err = countIndex(c.esClient, metricIndex)
	// 404 is acceptable in this scenario, as the index may not exist yet.
	if err != nil && !client.IsNotFound(err) {
		return err
	}
	c.metricIndexCount = count

	count, err = countIndex(c.esClient, errorIndex)
	// 404 is acceptable in this scenario, as the index may not exist yet.
	if err != nil && !client.IsNotFound(err) {
		return err
	}
	c.errorIndexCount = count

	ctx, cancel := context.WithTimeout(context.Background(), DefaultReqTimeout)
	defer cancel()
	eventsErrorResponse, err := c.apmClient.IntakeV2Events(ctx, false, []byte(sampleEventBody))
	if err != nil {
		return err
	}

	// in the happy case, we get no error response
	if eventsErrorResponse != nil {
		return fmt.Errorf("expected no error response when sending event to apm server got: %v", *eventsErrorResponse)
	}

	return nil
}

func assertHTTP403(t assert.TestingT, err error, msgAndArgs ...interface{}) bool {
	if !commonhttp.IsForbidden(err) {
		return assert.Fail(t, fmt.Sprintf("expected HTTP 403 but was %+v", err), msgAndArgs)
	}
	return true
}

func (c *apmClusterChecks) CheckRUMEventsAPI(rumEnabled bool) test.Step {
	sampleBody := `{"metadata":{"service":{"name":"apm-agent-js","version":"1.0.0","agent":{"name":"rum-js","version":"0.0.0"}}}}
{"transaction":{"id":"611f4fa950f04631","type":"page-load","duration":643,"context":{"page":{"referer":"http://localhost:8000/test/e2e/","url":"http://localhost:8000/test/e2e/general-usecase/"}},"trace_id":"611f4fa950f04631aaaaaaaaaaaaaaaa","span_count":{"started":1}}}`

	should := "forbidden"
	assertApplicationError := assert.NotNil
	assertRequestError := assertHTTP403
	if rumEnabled {
		should = "accepted"
		assertApplicationError = assert.Nil
		assertRequestError = assert.NoError
	}
	//nolint:thelper
	return test.Step{
		Name: "Events should be " + should,
		Test: func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), DefaultReqTimeout)
			defer cancel()
			eventsErrorResponse, err := c.apmClient.IntakeV2Events(ctx, true, []byte(sampleBody))
			assertRequestError(t, err)
			assertApplicationError(t, eventsErrorResponse)
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
func (c *apmClusterChecks) checkEventsInElasticsearch(apm apmv1.ApmServer, k *test.K8sClient) error {
	var updatedApmServer apmv1.ApmServer
	if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&apm), &updatedApmServer); err != nil {
		return err
	}

	if !updatedApmServer.Spec.ElasticsearchRef.IsDefined() {
		// No ES is referenced, do not try to check data
		return nil
	}

	metricIndex, errorIndex, err := getIndexNames(updatedApmServer)
	if err != nil {
		return err
	}

	if err := assertCountIndexEqual(c.esClient, metricIndex, c.metricIndexCount+1); err != nil {
		return err
	}

	return assertCountIndexEqual(c.esClient, errorIndex, c.errorIndexCount+1)
}

// getIndexNames will return the names of the metric, and error indexes, depending on
// the version of the APM Server, and any error encountered while parsing the version.
func getIndexNames(apm apmv1.ApmServer) (string, string, error) {
	var metricIndex, errorIndex string
	v, err := version.Parse(apm.Spec.Version)
	if err != nil {
		return metricIndex, errorIndex, err
	}

	// Check that the metric and error have been stored
	// default to indices names from 6.x
	metricIndex = fmt.Sprintf("apm-%s-2017.05.30", apm.EffectiveVersion())
	errorIndex = fmt.Sprintf("apm-%s-2018.08.09", apm.EffectiveVersion())
	switch v.Major {
	case 7:
		metricIndex = fmt.Sprintf("apm-%s-metric-2017.05.30", apm.EffectiveVersion())
		errorIndex = fmt.Sprintf("apm-%s-error-2018.08.09", apm.EffectiveVersion())
	case 8:
		// these are datastreams and not indices, but can be searched/counted in the same way
		metricIndex = "metrics-apm.app.1234_service_12a3-default"
		errorIndex = "logs-apm.error-default"
	}

	return metricIndex, errorIndex, nil
}

// assertCountIndexEqual asserts that the number of document in an index is the expected one, it raises an error otherwise.
func assertCountIndexEqual(esClient client.Client, index string, expected int) error {
	metricCount, err := countIndex(esClient, index)
	if err != nil {
		return err
	}
	if metricCount != expected {
		return fmt.Errorf("%d documents expected in index %s, got %d instead", expected, index, metricCount)
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
	body, err := io.ReadAll(response.Body)
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
				_, _, err = kibana.DoRequest(k, kb, password, "PUT", uri, []byte(sampleDefaultAgentConfiguration), http.Header{})
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

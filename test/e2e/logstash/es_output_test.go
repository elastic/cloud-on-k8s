// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build logstash || e2e

package logstash

import (
	"fmt"
	"strconv"
	"testing"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/logstash"
)

// TestLogstashEsOutput Logstash ingest events to Elasticsearch. Metrics should have `events.out` > 0.
func TestLogstashEsOutput(t *testing.T) {

	es := elasticsearch.NewBuilderWithoutSuffix("test-es").
		WithESMasterDataNodes(2, elasticsearch.DefaultResources)

	b := logstash.NewBuilder("test-ls-es-out").
		WithNodeCount(1).
		WithPipelines([]commonv1.Config{
			{
				Data: map[string]interface{}{
					"pipeline.id": "main",
					"config.string": `
input { exec { command => 'uptime' interval => 10 } } 
output { 
  elasticsearch {
	hosts => [ "${PRODUCTION_ES_HOSTS}" ]
	ssl => true
	cacert => "${PRODUCTION_ES_SSL_CERTIFICATE_AUTHORITY}"
	user => "${PRODUCTION_ES_USER}"
	password => "${PRODUCTION_ES_PASSWORD}"
  } 
}
`,
				},
			},
		}).
		WithElasticsearchRefs(
			logstashv1alpha1.ElasticsearchCluster{
				ObjectSelector: es.Ref(),
				ClusterName:    "production",
			},
		)

	steps := test.StepsFunc(func(k *test.K8sClient) test.StepList {
		return test.StepList{
			b.CheckMetricsRequest(k,
				logstash.Request{
					Name: "stats events",
					Path: "/_node/stats/events",
				},
				logstash.Want{
					MatchFunc: map[string]func(string) bool{
						// number of events goes out should be > 0
						"events.out": func(cntStr string) bool {
							cnt, err := strconv.Atoi(cntStr)
							if err != nil {
								fmt.Printf("failed to convert string %s to int", cntStr)
								return false
							}

							return cnt > 0
						},
					},
				}),
		}
	})

	test.Sequence(nil, steps, es, b).RunSequential(t)
}

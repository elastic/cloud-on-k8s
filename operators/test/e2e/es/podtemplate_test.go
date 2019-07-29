// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package es

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test/elasticsearch"
	"github.com/magiconair/properties/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCustomConfiguration(t *testing.T) {
	const synonyms = "synonyms"
	synonymMap := corev1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:      synonyms,
			Namespace: test.Namespace,
		},
		Data: map[string]string{
			"synonyms.txt": `ECK => Elastic Cloud on Kubernetes`,
		},
	}
	es := elasticsearch.NewBuilder(synonyms).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithPodTemplate(corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: synonyms,
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: synonyms,
								},
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name:      v1alpha1.ElasticsearchContainerName,
						Resources: elasticsearch.DefaultResources,
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      synonyms,
								MountPath: "/usr/share/elasticsearch/config/dictionaries",
							},
						},
					},
				},
			},
		}).
		WithRestrictedSecurityContext()

	init := func(c *test.K8sClient) test.StepList {
		return test.StepList{
			test.Step{
				Name: "Create dictionary config map",
				Test: func(t *testing.T) {
					// delete left over data from previous runs
					_ = c.Client.Delete(&synonymMap)
					require.NoError(t, c.Client.Create(&synonymMap))
				},
			},
		}
	}
	var esClient client.Client
	runAnaylzer := func(c *test.K8sClient) test.StepList {
		return test.StepList{
			test.Step{
				Name: "Create ES client",
				Test: func(t *testing.T) {
					var err error
					esClient, err = elasticsearch.NewElasticsearchClient(es.Elasticsearch, c)
					require.NoError(t, err)
				},
			},
			test.Step{
				Name: "Create index with synonym token filter",
				Test: func(t *testing.T) {
					settings := `
{
    "settings": {
        "index" : {
            "analysis" : {
                "analyzer" : {
                    "synonym" : {
                        "tokenizer" : "whitespace",
                        "filter" : ["synonym"]
                    }
                },
                "filter" : {
                    "synonym" : {
                        "type" : "synonym",
                        "synonyms_path" : "dictionaries/synonyms.txt"
                    }
                }
            }
        }
    }
}
`
					r, err := http.NewRequest(http.MethodPut, "/test-index", bytes.NewBufferString(settings))
					require.NoError(t, err)
					response, err := esClient.Request(context.Background(), r)
					defer response.Body.Close() // nolint
					require.NoError(t, err)
				},
			},
			{
				Name: "Analyse with synonyms",
				Test: func(t *testing.T) {
					body := `
{
  "analyzer": "synonym", 
  "text" : "ECK runs Elasticsearch, Kibana and APM Server on Kubernetes"
}
`
					r, err := http.NewRequest(http.MethodGet, "/test-index/_analyze", bytes.NewBufferString(body))
					require.NoError(t, err)
					response, err := esClient.Request(context.Background(), r)
					defer response.Body.Close() // nolint
					require.NoError(t, err)
					actual, err := ioutil.ReadAll(response.Body)
					require.NoError(t, err)
					expected := `{"tokens":[{"token":"Elastic","start_offset":0,"end_offset":3,"type":"SYNONYM","position":0},{"token":"runs","start_offset":4,"end_offset":8,"type":"word","position":1},{"token":"Cloud","start_offset":4,"end_offset":8,"type":"SYNONYM","position":1},{"token":"Elasticsearch,","start_offset":9,"end_offset":23,"type":"word","position":2},{"token":"on","start_offset":9,"end_offset":23,"type":"SYNONYM","position":2},{"token":"Kibana","start_offset":24,"end_offset":30,"type":"word","position":3},{"token":"Kubernetes","start_offset":24,"end_offset":30,"type":"SYNONYM","position":3},{"token":"and","start_offset":31,"end_offset":34,"type":"word","position":4},{"token":"APM","start_offset":35,"end_offset":38,"type":"word","position":5},{"token":"Server","start_offset":39,"end_offset":45,"type":"word","position":6},{"token":"on","start_offset":46,"end_offset":48,"type":"word","position":7},{"token":"Kubernetes","start_offset":49,"end_offset":59,"type":"word","position":8}]}`
					assert.Equal(t, string(actual), expected)
				},
			},
		}
	}
	test.Sequence(init, runAnaylzer, es).RunSequential(t)
}

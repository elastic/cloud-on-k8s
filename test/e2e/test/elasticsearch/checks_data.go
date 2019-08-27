// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"strings"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/go-test/deep"
)

type DataIntegrityCheck struct {
	clientFactory func() (client.Client, error) // recreate clients for cases where we switch scheme in tests
	indexName     string
	numShards     int
	numReplicas   int
	sampleData    map[string]interface{}
	docCount      int
}

func NewDataIntegrityCheck(k *test.K8sClient, b Builder) *DataIntegrityCheck {
	return &DataIntegrityCheck{
		clientFactory: func() (client.Client, error) {
			return NewElasticsearchClient(b.Elasticsearch, k)
		},
		indexName: "data-integrity-check",
		sampleData: map[string]interface{}{
			"foo": "bar",
		},
		docCount:    5,
		numShards:   3,
		numReplicas: dataIntegrityReplicas(b),
	}
}

func (dc *DataIntegrityCheck) Init() error {
	esClient, err := dc.clientFactory()
	if err != nil {
		return err
	}
	indexSettings := `
{
    "settings" : {
        "index" : {
            "number_of_shards" : %d,
            "number_of_replicas" : %d
        }
    }
}
`
	// create the index with controlled settings
	indexCreation, err := http.NewRequest(
		http.MethodPut,
		fmt.Sprintf("/%s", dc.indexName),
		bytes.NewBufferString(fmt.Sprintf(indexSettings, dc.numShards, dc.numReplicas)),
	)
	if err != nil {
		return err
	}
	resp, err := esClient.Request(context.Background(), indexCreation)
	if err != nil {
		return err
	}
	defer resp.Body.Close() // nolint

	// index a number of sample documents
	payload, err := json.Marshal(dc.sampleData)
	if err != nil {
		return err
	}
	for i := 0; i < dc.docCount; i++ {
		r, err := http.NewRequest(http.MethodPut, fmt.Sprintf("/%s/_doc/%d?refresh=true", dc.indexName, i), bytes.NewReader(payload))
		if err != nil {
			return err
		}
		resp, err = esClient.Request(context.Background(), r)
		if err != nil {
			return err
		}
		defer resp.Body.Close() // nolint
	}
	return nil
}

func (dc *DataIntegrityCheck) Verify() error {
	esClient, err := dc.clientFactory()
	if err != nil {
		return err
	}

	// retrieve the previously indexed documents
	r, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/%s/_search", dc.indexName), nil)
	if err != nil {
		return err
	}
	response, err := esClient.Request(context.Background(), r)
	if err != nil {
		return err
	}
	defer response.Body.Close() // nolint
	var results client.SearchResults
	resultBytes, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(resultBytes, &results)
	if err != nil {
		return err
	}
	// the overall count should be the same
	if len(results.Hits.Hits) != dc.docCount {
		return fmt.Errorf("expected %d got %d, data loss", dc.docCount, len(results.Hits.Hits))
	}
	// each document should be identical with the sample we used to create the test data
	for _, h := range results.Hits.Hits {
		if diff := deep.Equal(dc.sampleData, h.Source); diff != nil {
			return errors.New(strings.Join(diff, ", "))
		}
	}
	return nil
}

// dataIntegrityReplicas returns the number of replicas to use for the data integrity check,
// according to the cluster topology, since it affects the cluster health during the mutation.
func dataIntegrityReplicas(b Builder) int {
	initial := b.MutatedFrom
	if initial == nil {
		initial = &b // consider mutated == initial
	}

	if initial.Elasticsearch.Spec.NodeCount() == 1 || b.Elasticsearch.Spec.NodeCount() == 1 {
		// a 1 node cluster can only be green if shards have no replicas
		return 0
	}

	// attempt do detect a rolling upgrade scenario
	// Important: this only checks ES version and spec, other changes such as secure settings update
	// are tricky to capture and ignored here.
	isVersionUpgrade := initial.Elasticsearch.Spec.Version != b.Elasticsearch.Spec.Version
	httpOptionsChange := reflect.DeepEqual(initial.Elasticsearch.Spec.HTTP, b.Elasticsearch.Spec.HTTP)
	for _, initialNs := range initial.Elasticsearch.Spec.Nodes {
		for _, mutatedNs := range b.Elasticsearch.Spec.Nodes {
			if initialNs.Name == mutatedNs.Name &&
				(isVersionUpgrade || httpOptionsChange || !reflect.DeepEqual(initialNs, mutatedNs)) {
				// a rolling upgrade is scheduled for that NodeSpec
				// we need at least 1 replica per shard for the cluster to remain green during the operation
				return 1
			}
		}
	}

	// default to 0 replicas, to ensure proper data migration happens during the mutation
	return 0
}

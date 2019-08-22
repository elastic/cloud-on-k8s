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
	"strings"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/go-test/deep"
)

type DataIntegrityCheck struct {
	client     client.Client
	indexName  string
	numShards  int
	sampleData map[string]interface{}
	docCount   int
}

func NewDataIntegrityCheck(es v1alpha1.Elasticsearch, k *test.K8sClient) (*DataIntegrityCheck, error) {
	elasticsearchClient, err := NewElasticsearchClient(es, k)
	if err != nil {
		return nil, err
	}
	return &DataIntegrityCheck{
		client:    elasticsearchClient,
		indexName: "data-integrity-check",
		sampleData: map[string]interface{}{
			"foo": "bar",
		},
		docCount:  5,
		numShards: 3,
	}, nil
}

func (dc *DataIntegrityCheck) Init() error {
	// default to 0 replicas to ensure we test data migration works
	indexSettings := `
{
    "settings" : {
        "index" : {
            "number_of_shards" : %d,
            "number_of_replicas" : 0
        }
    }
}
`
	// create the index with controlled settings
	indexCreation, err := http.NewRequest(
		http.MethodPut,
		fmt.Sprintf("/%s", dc.indexName),
		bytes.NewBufferString(fmt.Sprintf(indexSettings, dc.numShards)),
	)
	if err != nil {
		return err
	}
	resp, err := dc.client.Request(context.Background(), indexCreation)
	defer resp.Body.Close() // nolint
	if err != nil {
		return err
	}

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
		resp, err = dc.client.Request(context.Background(), r)
		defer resp.Body.Close() // nolint
		if err != nil {
			return err
		}
	}
	return nil
}

func (dc *DataIntegrityCheck) Verify() error {
	// retrieve the previously indexed documents
	r, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/%s/_search", dc.indexName), nil)
	if err != nil {
		return err
	}
	response, err := dc.client.Request(context.Background(), r)
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

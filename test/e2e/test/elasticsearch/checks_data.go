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

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/go-test/deep"
)

const (
	DataIntegrityIndex = "data-integrity-check"
)

type DataIntegrityCheck struct {
	clientFactory       func() (client.Client, error) // recreate clients for cases where we switch scheme in tests
	indexName           string
	createIndexSettings createIndexSettings
	sampleData          map[string]interface{}
	docCount            int
}

func NewDataIntegrityCheck(k *test.K8sClient, b Builder) *DataIntegrityCheck {
	return &DataIntegrityCheck{
		clientFactory: func() (client.Client, error) {
			return NewElasticsearchClient(b.Elasticsearch, k)
		},
		indexName: DataIntegrityIndex,
		sampleData: map[string]interface{}{
			"foo": "bar",
		},
		docCount: 5,
		createIndexSettings: createIndexSettings{
			IndexSettings{
				NumberOfShards:   3,
				NumberOfReplicas: dataIntegrityReplicas(b),
			},
		},
	}
}

func (dc *DataIntegrityCheck) WithSoftDeletesEnabled(value bool) *DataIntegrityCheck {
	dc.createIndexSettings.SoftDeletesEnabled = &value
	return dc
}

type createIndexSettings struct {
	IndexSettings `json:"settings"`
}

type IndexSettings struct {
	NumberOfShards     int   `json:"number_of_shards"`
	NumberOfReplicas   int   `json:"number_of_replicas"`
	SoftDeletesEnabled *bool `json:"soft_deletes.enabled,omitempty"`
}

func (dc *DataIntegrityCheck) ForIndex(indexName string) *DataIntegrityCheck {
	dc.indexName = indexName
	return dc
}

func (dc *DataIntegrityCheck) Init() error {
	esClient, err := dc.clientFactory()
	if err != nil {
		return err
	}
	createIndexSettings, err := json.Marshal(dc.createIndexSettings)
	if err != nil {
		return err
	}

	// delete index if running check multiple times
	indexDeletion, err := http.NewRequest(
		http.MethodDelete,
		fmt.Sprintf("/%s", dc.indexName),
		nil,
	)
	if err != nil {
		return err
	}
	// delete the index but ignore errors (e.g. if it did not exist yet)
	resp, err := esClient.Request(context.Background(), indexDeletion)
	if err == nil {
		resp.Body.Close()
	}

	// create the index with controlled settings
	indexCreation, err := http.NewRequest(
		http.MethodPut,
		fmt.Sprintf("/%s", dc.indexName),
		bytes.NewBuffer(createIndexSettings),
	)
	if err != nil {
		return err
	}
	resp, err = esClient.Request(context.Background(), indexCreation)
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
	r, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/%s/_search?size=%d", dc.indexName, dc.docCount), nil)
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
	if MustNumDataNodes(initial.Elasticsearch) == 1 || MustNumDataNodes(b.Elasticsearch) == 1 {
		// a 1 node cluster can only be green if shards have no replicas
		return 0
	}
	if b.TriggersRollingUpgrade() {
		// a rolling upgrade will happen during the mutation: nodes will go down
		// we need at least 1 replica per shard for the cluster to remain green during the operation
		return 1
	}
	// default to 0 replicas, to ensure proper data migration happens during the mutation
	return 0
}

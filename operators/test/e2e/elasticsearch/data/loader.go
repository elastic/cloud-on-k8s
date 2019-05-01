// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package data

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/elastic/go-elasticsearch/esapi"
	estype "github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/test/e2e/helpers"
	"github.com/stretchr/testify/require"
)

const (
	indexName = "e2e-index"
	mapping   = `{
    "settings" : {
        "number_of_shards" : %d,
		"number_of_replicas" : %d
    },
    "mappings": {
        "properties": {
          "id":         { "type": "keyword" },
          "creation":  { "type": "date" },
          "name":       { "type": "keyword"}
        }
    }
}`
)

// Loader can be used to create a sample index, bulk import some data and check if all the data
// are available during some e2e tests.
type Loader struct {
	es        estype.Elasticsearch
	k8sHelper *helpers.K8sHelper
	// Creation of the client is delayed because we must way for the secrets to be created.
	Client *helpers.Client
	// Number of replica
	replicas int
	// Number of shards
	shards int
	// Expected documents
	expected int
}

// NewDataLoader creates a new data loader. The returned implementation is not thread-safe.
func NewDataLoader(es estype.Elasticsearch, k *helpers.K8sHelper, replicas, shards int) *Loader {
	return &Loader{
		es:        es,
		k8sHelper: k,
		replicas:  replicas,
		shards:    shards,
	}
}

func (d *Loader) init() error {
	if d.Client != nil {
		return nil
	}
	var err error
	d.Client, err = helpers.NewElasticsearchClient(d.es, d.k8sHelper)
	return err
}

func generateData(count int) []sampleDocument {
	documents := make([]sampleDocument, count)
	for i := 0; i < count; i++ {
		id := i + 1
		documents[i] = sampleDocument{
			ID:       id,
			Name:     fmt.Sprintf("Document %d", id),
			Creation: time.Now().Round(time.Second).UTC(),
			Type:     "document",
		}
	}
	return documents
}

// Load inserts documents with the /bulk endpoint.
func (d *Loader) Load(count int) error {
	if err := d.init(); err != nil {
		return err
	}
	documents := generateData(count)

	// Always make sure that the index has been created
	if err := d.ensureIndex(); err != nil {
		return err
	}

	var buf bytes.Buffer
	for _, doc := range documents {
		meta := []byte(fmt.Sprintf(`{ "index" : { "_type": "_doc", "_index":"%s", "_id" : "%d" } }%s`, indexName, doc.ID, "\n"))
		data, err := json.Marshal(doc)
		if err != nil {
			return err
		}
		data = append(data, "\n"...)
		buf.Grow(len(meta) + len(data))
		buf.Write(meta)
		buf.Write(data)
	}
	body := buf.Bytes()
	res, err := d.Client.Bulk(bytes.NewReader(body),
		d.Client.Bulk.WithIndex(indexName),
		d.Client.Bulk.WithRefresh("true"),
	)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.IsError() {
		return getErrorFromResponse(res)
	}

	var blk *bulkResponse
	if err := json.NewDecoder(res.Body).Decode(&blk); err != nil {
		return err
	}
	// A successful response might still contain errors for particular documents.
	for _, d := range blk.Items {
		if d.Index.Status > 201 {
			return fmt.Errorf("bulk loading error: [%d]: %s: %s: %s: %s",
				d.Index.Status,
				d.Index.Error.Type,
				d.Index.Error.Reason,
				d.Index.Error.Cause.Type,
				d.Index.Error.Cause.Reason,
			)
		}
	}
	// Increment the expected document in the cluster
	d.expected += count
	return nil
}

// CheckData gets the current number of documents in the e2e index and compares it with the expected one.
func (d *Loader) CheckData(t *testing.T) {
	require.NoError(t, d.init())
	res, err := d.Client.Count(d.Client.Count.WithIndex(indexName))
	defer res.Body.Close()
	require.NoError(t, err)
	require.NoError(t, getErrorFromResponse(res))

	var r countResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&r))
	require.Equal(t, d.expected, r.Count)
}

func getErrorFromResponse(res *esapi.Response) error {
	if res == nil {
		return nil
	}
	if res.IsError() {
		var raw map[string]interface{}
		if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
			return err
		}
		return fmt.Errorf("  Error: [%d] %s: %s",
			res.StatusCode,
			raw["error"].(map[string]interface{})["type"],
			raw["error"].(map[string]interface{})["reason"],
		)
	}
	return nil
}

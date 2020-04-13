// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package migration

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"path/filepath"

	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
)

func loadFileBytes(fileName string) []byte {
	contents, err := ioutil.ReadFile(filepath.Join("testdata", fileName))
	if err != nil {
		panic(err)
	}

	return contents
}

type fakeClient struct {
	esclient.Client
	exclusions string
	shards     esclient.Shards
	err        error
}

func NewFakeShardLister(shards esclient.Shards) esclient.ShardLister {
	return &fakeClient{shards: shards}
}

func NewFakeShardListerWithError(shards esclient.Shards, err error) esclient.ShardLister {
	return &fakeClient{shards: shards, err: err}
}

func NewFakeShardFromFile(fileName string) esclient.ShardLister {
	var cs esclient.Shards
	sampleClusterState := loadFileBytes(fileName)
	err := json.Unmarshal(sampleClusterState, &cs)
	return &fakeClient{shards: cs, err: err}
}

func (f *fakeClient) ExcludeFromShardAllocation(_ context.Context, nodes string) error {
	f.exclusions = nodes
	return nil
}

func (f *fakeClient) ExcludedFromShardAllocation(_ context.Context) (string, error) {
	return f.exclusions, f.err
}

func (f *fakeClient) GetShards(_ context.Context) (esclient.Shards, error) {
	return f.shards, f.err
}

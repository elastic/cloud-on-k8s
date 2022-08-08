// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package migration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	esclient "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
)

func loadFileBytes(fileName string) []byte {
	contents, err := os.ReadFile(filepath.Join("testdata", fileName))
	if err != nil {
		panic(err)
	}

	return contents
}

type fakeShardLister struct {
	shards           esclient.Shards
	hasShardActivity bool
	err              error
}

func (f *fakeShardLister) HasShardActivity(_ context.Context) (bool, error) {
	return f.hasShardActivity, nil
}

func (f *fakeShardLister) GetShards(_ context.Context) (esclient.Shards, error) {
	return f.shards, f.err
}

func NewFakeShardListerWithShardActivity(shards esclient.Shards) esclient.ShardLister {
	return &fakeShardLister{
		shards:           shards,
		hasShardActivity: true,
	}
}

func NewFakeShardLister(shards esclient.Shards) esclient.ShardLister {
	return &fakeShardLister{shards: shards}
}

func NewFakeShardListerWithError(shards esclient.Shards, err error) esclient.ShardLister {
	return &fakeShardLister{shards: shards, err: err}
}

func NewFakeShardFromFile(fileName string) esclient.ShardLister {
	var cs esclient.Shards
	sampleClusterState := loadFileBytes(fileName)
	err := json.Unmarshal(sampleClusterState, &cs)
	return &fakeShardLister{shards: cs, err: err}
}

type fakeAllocationSetter struct {
	value string
}

func (f *fakeAllocationSetter) ExcludeFromShardAllocation(_ context.Context, nodes string) error {
	f.value = nodes
	return nil
}

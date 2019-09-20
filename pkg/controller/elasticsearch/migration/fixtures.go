package migration

import (
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

type fakeShardLister struct {
	shards esclient.Shards
	err    error
}

func (f *fakeShardLister) GetShards() (esclient.Shards, error) {
	return f.shards, f.err
}

func NewFakeShardLister(shards esclient.Shards) esclient.ShardLister {
	return &fakeShardLister{shards: shards}
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

func (f *fakeAllocationSetter) ExcludeFromShardAllocation(nodes string) error {
	f.value = nodes
	return nil
}

package elasticsearch

import (
	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/client"
	"k8s.io/api/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("migrate-data")

//IsMigrating Data looks only at the presence of shards on a given node
// TODO check global allocation filters
func IsMigratingData(c *client.Client, pod v1.Pod) (bool, error) {
	shards, e := c.CatShards()
	if e != nil {
		return true, e
	}

	for _, shard := range shards {
		if shard.Node == pod.Name {
			return true, nil
		}
	}

	return false, nil
}

//MigrateData sets allocation filters for the given pod
func MigrateData(client *client.Client, pod v1.Pod) error {
	//update allocation exclusions
	e := client.ExcludeFromShardAllocation(pod.Name)
	return e
}

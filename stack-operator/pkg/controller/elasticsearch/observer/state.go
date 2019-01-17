package observer

import (
	"context"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
	esclient "github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
)

// State contains information about an observed state of Elasticsearch.
type State struct {
	// TODO: verify usages of the two below never assume they are set (check for nil)

	// ClusterState is the current Elasticsearch cluster state if any.
	ClusterState *esclient.ClusterState
	// ClusterHealth is the current traffic light health as reported by Elasticsearch.
	ClusterHealth *esclient.Health
}

// RetrieveState returns the current Elasticsearch cluster state
func RetrieveState(ctx context.Context, esClient *esclient.Client) State {
	// retrieve both cluster state and health in parallel
	clusterStateChan := make(chan *client.ClusterState)
	healthChan := make(chan *client.Health)

	go func() {
		clusterState, err := esClient.GetClusterState(ctx)
		if err != nil {
			log.Info("Unable to retrieve cluster state", "error", err.Error())
			clusterStateChan <- nil
			return
		}
		clusterStateChan <- &clusterState
	}()

	go func() {
		health, err := esClient.GetClusterHealth(ctx)
		if err != nil {
			log.Info("Unable to retrieve cluster health", "error", err.Error())
			healthChan <- nil
			return
		}
		healthChan <- &health
	}()

	// return the state when ready, may contain nil values
	return State{
		ClusterHealth: <-healthChan,
		ClusterState:  <-clusterStateChan,
	}
}

package observer

import (
	"context"

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
	state := State{}

	clusterState, err := esClient.GetClusterState(ctx)
	if err != nil {
		log.Info("Unable to retrieve cluster state, continuing", "error", err.Error())
	} else {
		state.ClusterState = &clusterState
	}

	// TODO: if the above errored, we might want to consider bailing? or do the requests in parallel

	// TODO we could derive cluster health from the routing table and save this request
	health, err := esClient.GetClusterHealth(ctx)
	if err != nil {
		// don't log this as error as this is expected when cluster is forming etc.
		log.Info("Unable to retrieve cluster health, continuing", "error", err.Error())
	} else {
		state.ClusterHealth = &health
	}

	return state
}

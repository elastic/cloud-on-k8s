// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package observer

import (
	"context"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	esclient "github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
)

// State contains information about an observed state of Elasticsearch.
type State struct {
	// TODO: verify usages of the two below never assume they are set (check for nil)

	// ClusterState is the current Elasticsearch cluster state if any.
	ClusterState *esclient.ClusterState
	// ClusterHealth is the current traffic light health as reported by Elasticsearch.
	ClusterHealth *esclient.Health
	// TODO should probably be a separate observer
	// ClusterLicense is the current license applied to this cluster
	ClusterLicense *esclient.License
}

// RetrieveState returns the current Elasticsearch cluster state
func RetrieveState(ctx context.Context, esClient esclient.Client) State {
	// retrieve both cluster state and health in parallel
	clusterStateChan := make(chan *client.ClusterState)
	healthChan := make(chan *client.Health)
	licenseChan := make(chan *client.License)

	go func() {
		clusterState, err := esClient.GetClusterState(ctx)
		if err != nil {
			log.V(3).Info("Unable to retrieve cluster state", "error", err)
			clusterStateChan <- nil
			return
		}
		clusterStateChan <- &clusterState
	}()

	go func() {
		health, err := esClient.GetClusterHealth(ctx)
		if err != nil {
			log.V(3).Info("Unable to retrieve cluster health", "error", err)
			healthChan <- nil
			return
		}
		healthChan <- &health
	}()

	go func() {
		license, err := esClient.GetLicense(ctx)
		if err != nil {
			log.V(3).Info("Unable to retrieve cluster license", "error", err)
			licenseChan <- nil
			return
		}
		licenseChan <- &license
	}()

	// return the state when ready, may contain nil values
	return State{
		ClusterHealth:  <-healthChan,
		ClusterState:   <-clusterStateChan,
		ClusterLicense: <-licenseChan,
	}
}

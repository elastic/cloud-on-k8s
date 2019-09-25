// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package observer

import (
	"context"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"k8s.io/apimachinery/pkg/types"
)

// State contains information about an observed state of Elasticsearch.
type State struct {
	// TODO: verify usages of the two below never assume they are set (check for nil)
	// ClusterInfo is mostly used to retrieve the cluster UUID.
	ClusterInfo *esclient.Info
	// ClusterHealth is the current traffic light health as reported by Elasticsearch.
	ClusterHealth *esclient.Health
	// TODO should probably be a separate observer
	// ClusterLicense is the current license applied to this cluster
	ClusterLicense *esclient.License
}

// RetrieveState returns the current Elasticsearch cluster state
func RetrieveState(ctx context.Context, cluster types.NamespacedName, esClient esclient.Client) State {
	// retrieve cluster info, health and license in parallel
	infoChan := make(chan *client.Info)
	healthChan := make(chan *client.Health)
	licenseChan := make(chan *client.License)

	go func() {
		info, err := esClient.GetClusterInfo(ctx)
		if err != nil {
			log.V(1).Info("Unable to retrieve cluster info", "error", err, "namespace", cluster.Namespace, "es_name", cluster.Name)
			infoChan <- nil
			return
		}
		infoChan <- &info
	}()

	go func() {
		health, err := esClient.GetClusterHealth(ctx)
		if err != nil {
			log.V(1).Info("Unable to retrieve cluster health", "error", err, "namespace", cluster.Namespace, "es_name", cluster.Name)
			healthChan <- nil
			return
		}
		healthChan <- &health
	}()

	go func() {
		license, err := esClient.GetLicense(ctx)
		if err != nil {
			log.V(1).Info("Unable to retrieve cluster license", "error", err, "namespace", cluster.Namespace, "es_name", cluster.Name)
			licenseChan <- nil
			return
		}
		licenseChan <- &license
	}()

	// return the state when ready, may contain nil values
	return State{
		ClusterInfo:    <-infoChan,
		ClusterHealth:  <-healthChan,
		ClusterLicense: <-licenseChan,
	}
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package observer

import (
	"context"

	"k8s.io/apimachinery/pkg/types"

	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
)

// State contains information about an observed state of Elasticsearch.
type State struct {
	// TODO: verify usages of the below never assume they are set (check for nil)
	// ClusterHealth is the current traffic light health as reported by Elasticsearch.
	ClusterHealth *esclient.Health
}

// RetrieveState returns the current Elasticsearch cluster state
func RetrieveState(ctx context.Context, cluster types.NamespacedName, esClient esclient.Client) State {
	health, err := esClient.GetClusterHealth(ctx)
	if err != nil {
		log.V(1).Info("Unable to retrieve cluster health", "error", err, "namespace", cluster.Namespace, "es_name", cluster.Name)
		return State{ClusterHealth: nil}
	}
	return State{ClusterHealth: &health}
}

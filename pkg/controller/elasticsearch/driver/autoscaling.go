// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"context"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/autoscaling/elasticsearch/resources"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/autoscaling"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

// autoscaledResourcesSynced checks that the autoscaler controller has updated the resources
// when autoscaling is enabled. This is to avoid situations where resources have been manually
// deleted or replaced by an external event. The Elasticsearch controller should then wait for
// the Elasticsearch autoscaling controller to update again the resources in the NodeSets.
func (d *defaultDriver) autoscaledResourcesSynced(ctx context.Context, es esv1.Elasticsearch) (bool, error) {
	autoscalingResource, err := autoscaling.GetAssociatedAutoscalingResource(ctx, d.Client, es)
	if err != nil {
		return false, err
	}
	if autoscalingResource == nil {
		// Cluster is not managed by an autoscaler.
		return true, nil
	}

	log := ulog.FromContext(ctx)
	autoscalingSpecs, err := autoscalingResource.GetAutoscalingPolicySpecs()
	if err != nil {
		return false, err
	}
	autoscalingStatus, err := autoscalingResource.GetElasticsearchAutoscalerStatus()
	if err != nil {
		return false, err
	}
	v, err := version.Parse(es.Spec.Version)
	if err != nil {
		return false, err
	}
	for _, nodeSet := range es.Spec.NodeSets {
		nodeSetAutoscalingSpec, err := nodeSet.GetAutoscalingSpecFor(v, autoscalingSpecs)
		if err != nil {
			return false, err
		}
		if nodeSetAutoscalingSpec == nil {
			// This nodeSet is not managed by an autoscaling configuration
			log.V(1).Info("NodeSet not managed by an autoscaling controller", "nodeset", nodeSet.Name)
			continue
		}

		expectedNodeSetsResources, ok := autoscalingStatus.CurrentResourcesForPolicy(nodeSetAutoscalingSpec.Name)
		if !ok {
			log.Info("NodeSet managed by the autoscaling controller but not found in status",
				"nodeset", nodeSet.Name,
			)
			return false, nil
		}
		inSync, err := resources.Match(expectedNodeSetsResources, esv1.ElasticsearchContainerName, nodeSet)
		if err != nil {
			return false, err
		}
		if !inSync {
			log.Info("NodeSet managed by the autoscaling controller but not in sync",
				"nodeset", nodeSet.Name,
				"expected", expectedNodeSetsResources.NodeResources,
			)
			return false, nil
		}
	}

	return true, nil
}

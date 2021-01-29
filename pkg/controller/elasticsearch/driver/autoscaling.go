// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/autoscaling/elasticsearch/status"
)

// autoscaledResourcesSynced checks that the autoscaler controller has updated the resources
// when autoscaling is enabled. This is to avoid situations where resources have been manually
// deleted or replaced by an external event. The Elasticsearch controller should then wait for
// the Elasticsearch autoscaling controller to update again the resources in the NodeSets.
func autoscaledResourcesSynced(es esv1.Elasticsearch) (bool, error) {
	if !es.IsAutoscalingDefined() {
		return true, nil
	}
	autoscalingSpec, err := es.GetAutoscalingSpecification()
	if err != nil {
		return false, err
	}
	autoscalingStatus, err := status.GetStatus(es)
	if err != nil {
		return false, err
	}

	for _, nodeSet := range es.Spec.NodeSets {
		nodeSetAutoscalingSpec, err := autoscalingSpec.GetAutoscalingSpecFor(nodeSet)
		if err != nil {
			return false, err
		}
		if nodeSetAutoscalingSpec == nil {
			// This nodeSet is not managed by an autoscaling configuration
			log.V(1).Info("NodeSet not managed by an autoscaling controller", "nodeset", nodeSet.Name)
			continue
		}

		s, ok := autoscalingStatus.GetNamedTierResources(nodeSetAutoscalingSpec.Name)
		if !ok {
			log.Info("NodeSet managed by the autoscaling controller but not found in status",
				"nodeset", nodeSet.Name,
			)
			return false, nil
		}
		inSync, err := s.IsUsedBy(esv1.ElasticsearchContainerName, nodeSet)
		if err != nil {
			return false, err
		}
		if !inSync {
			log.Info("NodeSet managed by the autoscaling controller but not in sync",
				"nodeset", nodeSet.Name,
				"expected", s.NodeResources,
			)
			return false, nil
		}
	}

	return true, nil
}

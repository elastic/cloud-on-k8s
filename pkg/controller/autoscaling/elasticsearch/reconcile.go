// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"github.com/go-logr/logr"
	apiequality "k8s.io/apimachinery/pkg/api/equality"

	"github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
)

// reconcileElasticsearch updates Elasticsearch NodeSets according to autoscaling recommendations.
// It updates NodeSet count and the CPU/memory/storage shorthand resources.
//
// Every field written here is a granular leaf under spec.nodeSets[name=...], which keeps the
// operator's Server-Side Apply field ownership scoped to those leaves. In particular the storage
// size goes to Resources.Storage rather than into VolumeClaimTemplates: that list is atomic under
// SSA, since a PersistentVolumeClaim has no top-level scalar to key it by, so writing into it would
// claim the storage class and access modes along with the size.
func reconcileElasticsearch(
	log logr.Logger,
	es *esv1.Elasticsearch,
	nextClusterResources v1alpha1.ClusterResources,
) {
	nextResourcesByNodeSet := nextClusterResources.ByNodeSet()
	for i := range es.Spec.NodeSets {
		name := es.Spec.NodeSets[i].Name
		nodeSetResources, ok := nextResourcesByNodeSet[name]
		if !ok {
			// No desired resources returned for this NodeSet, leave it untouched.
			log.V(1).Info("Skipping nodeset update", "nodeset", name)
			continue
		}

		// Compute the next shorthand resources. During operator upgrades from versions that wrote
		// autoscaled CPU/memory only in the PodTemplate container resources, this progressively
		// converges NodeSet.Resources to the autoscaler recommendation.
		currentResources := es.Spec.NodeSets[i].Resources
		nextResources := esv1.NodeSetResources{
			Resources: nodeSetResources.NodeResources.ToNodeSetResourcesWith(currentResources.ContainerResources()),
			Storage:   nodeSetResources.NodeResources.StorageRequestOr(currentResources.Storage),
		}

		// Only write to the NodeSet when something changed so the upstream write (in the
		// controller) is a no-op and does not dirty the Elasticsearch custom resource
		// unnecessarily on every reconcile.
		if es.Spec.NodeSets[i].Count != nodeSetResources.NodeCount ||
			!apiequality.Semantic.DeepEqual(currentResources, nextResources) {
			es.Spec.NodeSets[i].Count = nodeSetResources.NodeCount
			es.Spec.NodeSets[i].Resources = nextResources
			log.V(1).Info("Updating nodeset with resources", "nodeset", name, "resources", nextClusterResources)
		}
	}
}

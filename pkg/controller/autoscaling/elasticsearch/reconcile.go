// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/autoscaling"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/volume"
)

// reconcileElasticsearch updates Elasticsearch NodeSets according to autoscaling recommendations.
// It updates NodeSet count and CPU/memory shorthand resources, and adjusts storage when needed.
func reconcileElasticsearch(
	log logr.Logger,
	es *esv1.Elasticsearch,
	nextClusterResources v1alpha1.ClusterResources,
) error {
	nextResourcesByNodeSet := nextClusterResources.ByNodeSet()
	for i := range es.Spec.NodeSets {
		name := es.Spec.NodeSets[i].Name
		nodeSetResources, ok := nextResourcesByNodeSet[name]
		if !ok {
			// No desired resources returned for this NodeSet, leave it untouched.
			log.V(1).Info("Skipping nodeset update", "nodeset", name)
			continue
		}

		// Create a copy to compare if some changes have been made.
		currentResources := es.Spec.NodeSets[i].Resources

		// Update desired count
		es.Spec.NodeSets[i].Count = nodeSetResources.NodeCount

		// Update CPU and memory shorthand resources.
		es.Spec.NodeSets[i].Resources = nodeSetResources.NodeResources.ToNodeSetResourcesWith(es.Spec.NodeSets[i].Resources)

		// Update storage
		if nodeSetResources.HasRequest(corev1.ResourceStorage) {
			nextStorage, err := newVolumeClaimTemplate(nodeSetResources.GetRequest(corev1.ResourceStorage), es.Spec.NodeSets[i])
			if err != nil {
				return err
			}
			es.Spec.NodeSets[i].VolumeClaimTemplates = nextStorage
		}

		if !apiequality.Semantic.DeepEqual(currentResources, es.Spec.NodeSets[i].Resources) {
			log.V(1).Info("Updating nodeset with resources", "nodeset", name, "resources", nextClusterResources)
		}
	}
	return nil
}

func newVolumeClaimTemplate(storageQuantity resource.Quantity, nodeSet esv1.NodeSet) ([]corev1.PersistentVolumeClaim, error) {
	onlyOneVolumeClaimTemplate, volumeClaimTemplate := autoscaling.HasAtMostOnePersistentVolumeClaim(nodeSet)
	if !onlyOneVolumeClaimTemplate {
		return nil, fmt.Errorf(autoscaling.UnexpectedVolumeClaimError)
	}
	if volumeClaimTemplate == nil {
		// Init a new volume claim template
		volumeClaimTemplate = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: volume.ElasticsearchDataVolumeName,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
			},
		}
	}
	// Adjust the size
	if volumeClaimTemplate.Spec.Resources.Requests == nil {
		volumeClaimTemplate.Spec.Resources.Requests = make(corev1.ResourceList)
	}
	volumeClaimTemplate.Spec.Resources.Requests[corev1.ResourceStorage] = storageQuantity
	return []corev1.PersistentVolumeClaim{*volumeClaimTemplate}, nil
}

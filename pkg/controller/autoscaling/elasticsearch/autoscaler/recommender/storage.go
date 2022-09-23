// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package recommender

import (
	"fmt"
	"math"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/volume"
)

type storage struct {
	base

	// hasZeroRequirement is true when ES returns a requirement set to 0, just return the min storage in that case
	hasZeroRequirement bool

	// All resources can be computed "from scratch", without knowing the previous values.
	// This is however not true for storage. Storage can't be scaled down, current storage capacity must be considered
	// as a hard min. limit. This storage limit must be taken into consideration when computing the desired resources.
	minNodeStorageSize resource.Quantity

	// The observed storage capacity as reported by Elasticsearch is mandatory as it is not possible for ECK
	// to accurately detect the storage capacity that can be used by Elasticsearch.
	observedTotalStorageCapacity, observedNodeStorageCapacity client.AutoscalingCapacity

	requiredNodeStorageCapacity, requiredTotalStorageCapacity *client.AutoscalingCapacity
}

func (s *storage) ManagedResource() corev1.ResourceName {
	return corev1.ResourceStorage
}

func (s *storage) HasResourceRecommendation() bool {
	return true
}

func (s *storage) NodeResourceQuantity() resource.Quantity {
	if s.hasZeroRequirement {
		return s.minNodeStorageSize
	}
	var storageRequest resource.Quantity
	// Elasticsearch does not support scale down for storage, always check if we should scale up first.
	// Otherwise return the current claimed storage capacity.
	if s.shouldScaleUp() {
		// Required capacity is greater than the one observed by Elasticsearch.
		storageRequest = getResourceValue(
			s.log,
			s.autoscalingSpec,
			s.statusBuilder,
			string(s.ManagedResource()),
			adjustRequiredStorage(s.requiredNodeStorageCapacity),
			adjustRequiredStorage(s.requiredTotalStorageCapacity),
			*s.autoscalingSpec.StorageRange,
		)
	} else {
		// No need to scale up, return the current claimed storage capacity.
		currentResources, _ := s.currentAutoscalingStatus.CurrentResourcesForPolicy(s.autoscalingSpec.Name)
		storageRequest = currentResources.GetRequest(corev1.ResourceStorage)
	}
	return maxResource(s.minNodeStorageSize, storageRequest)
}

// shouldScaleUp calculates if individual node storage capacity should be updated to match Elasticsearch requirements.
func (s *storage) shouldScaleUp() bool {
	currentResources, _ := s.currentAutoscalingStatus.CurrentResourcesForPolicy(s.autoscalingSpec.Name)
	resourceInStatus := currentResources.HasRequest(corev1.ResourceStorage)
	if !resourceInStatus {
		// No resource in status, ensure we compute a new one
		return true
	}
	currentClaimedStorage := currentResources.GetRequest(corev1.ResourceStorage)
	if s.base.autoscalingSpec.StorageRange.Min.Equal(s.base.autoscalingSpec.StorageRange.Max) &&
		s.base.autoscalingSpec.StorageRange.Min.Equal(currentClaimedStorage) {
		// User provided a singular range (min == max) and the current claimed capacity is already set to the correct value, no need to scale vertically.
		return false
	}
	if s.observedNodeStorageCapacity.Value() > currentClaimedStorage.Value() {
		// Log a warning and ensure we max out the storage in the claim to also scale up dependant resources like memory or CPU.
		s.log.Info(
			"Vertical Pod autoscaling is not supported: current node storage capacity is greater than the claimed capacity.",
			"policy", s.autoscalingSpec.Name,
			"scope", "node",
			"resource", "storage",
			"current_node_storage_capacity", s.observedNodeStorageCapacity.Value(),
			"current_claimed_storage_capacity", currentClaimedStorage.Value(),
		)

		// Also surface this situation in the status.
		s.statusBuilder.
			ForPolicy(s.autoscalingSpec.Name).
			RecordEvent(
				v1alpha1.UnexpectedNodeStorageCapacity,
				fmt.Sprintf(
					"Vertical Pod autoscaling is not supported: current node storage capacity %d is greater than the claimed capacity %d",
					s.observedNodeStorageCapacity.Value(),
					currentClaimedStorage.Value(),
				),
			)
		return true
	}
	return s.requiredNodeStorageCapacity.Value() > s.observedNodeStorageCapacity.Value() ||
		s.requiredTotalStorageCapacity.Value() > s.observedTotalStorageCapacity.Value()
}

func (s *storage) NodeCount(nodeCapacity v1alpha1.NodeResources) int32 {
	// A value of 0 explicitly means that storage decider should not prevent a scale down.
	// For example this could be the case for ML nodes when there is no ML jobs to run.
	if s.requiredTotalStorageCapacity.IsZero() {
		return s.autoscalingSpec.NodeCountRange.Enforce(0)
	}
	// Elasticsearch does not support data nodes scale down, always check if we should scale up first.
	// Otherwise return the current node count.
	currentResources, hasResources := s.currentAutoscalingStatus.CurrentResourcesForPolicy(s.autoscalingSpec.Name)
	if !hasResources || s.requiredTotalStorageCapacity.Value() > s.observedTotalStorageCapacity.Value() {
		nodeStorage := nodeCapacity.GetRequest(corev1.ResourceStorage)
		adjustedTotalRequiredCapacity := adjustRequiredStorage(s.requiredTotalStorageCapacity)
		return getNodeCount(
			s.log,
			s.autoscalingSpec,
			s.statusBuilder,
			string(s.ManagedResource()),
			nodeStorage.Value(),
			adjustedTotalRequiredCapacity.Value(),
		)
	}
	return s.autoscalingSpec.NodeCountRange.Enforce(currentResources.NodeSetNodeCount.TotalNodeCount())
}

func NewStorageRecommender(
	log logr.Logger,
	statusBuilder *v1alpha1.AutoscalingStatusBuilder,
	autoscalingSpec v1alpha1.AutoscalingPolicySpec,
	autoscalingPolicyResult client.AutoscalingPolicyResult,
	currentAutoscalingStatus v1alpha1.ElasticsearchAutoscalerStatus,
) (Recommender, error) {
	// Check if user expects the resource to be managed by the autoscaling controller
	hasResourceRange := autoscalingSpec.StorageRange != nil

	// Did we get a resource requirement from Elasticsearch ?
	hasRequirement := !autoscalingPolicyResult.RequiredCapacity.Node.Storage.IsEmpty() ||
		!autoscalingPolicyResult.RequiredCapacity.Total.Storage.IsEmpty()

	// We must recommend something in one of the following situations:
	// * User has provided a resource range for the resource.
	// * Elasticsearch API returned a non zero requirement
	if !hasResourceRange && !hasRequirement {
		return &nilRecommender{}, nil
	}

	// Ensure that we have all the information needed to recommend something.
	// In case of storage it means that we must have the observed node capacity.
	if autoscalingPolicyResult.CurrentCapacity.Total.Storage == nil ||
		autoscalingPolicyResult.CurrentCapacity.Node.Storage == nil {
		return nil, fmt.Errorf("observed storage capacity is expected in Elasticsearch autoscaling API response")
	}

	if autoscalingSpec.StorageRange == nil {
		statusBuilder.ForPolicy(autoscalingSpec.Name).RecordEvent(v1alpha1.StorageRequired, "Min and max storage must be specified")
		return nil, fmt.Errorf("min and max storage must be specified")
	}

	storageRecommender := storage{
		base: base{
			log:                      log,
			autoscalingSpec:          autoscalingSpec,
			statusBuilder:            statusBuilder,
			currentAutoscalingStatus: currentAutoscalingStatus,
		},
		hasZeroRequirement: autoscalingPolicyResult.RequiredCapacity.Node.Storage.IsZero() &&
			autoscalingPolicyResult.RequiredCapacity.Total.Storage.IsZero(),
		// In case of storage we must not scale down vertically the storage capacity
		minNodeStorageSize:           getMinStorageQuantity(autoscalingSpec, currentAutoscalingStatus),
		requiredTotalStorageCapacity: autoscalingPolicyResult.RequiredCapacity.Total.Storage,
		requiredNodeStorageCapacity:  autoscalingPolicyResult.RequiredCapacity.Node.Storage,
		// Observed storage capacity is retrieved from the Elasticsearch autoscaling response.
		observedNodeStorageCapacity:  *autoscalingPolicyResult.CurrentCapacity.Node.Storage,
		observedTotalStorageCapacity: *autoscalingPolicyResult.CurrentCapacity.Total.Storage,
	}

	return &storageRecommender, nil
}

// getMinStorageQuantity returns the min. storage quantity that should be used by the autoscaling algorithm.
// The value is the max. value of either:
// * the current value in the status
// * the min. value set by the user in the autoscaling spec.
func getMinStorageQuantity(autoscalingSpec v1alpha1.AutoscalingPolicySpec, currentAutoscalingStatus v1alpha1.ElasticsearchAutoscalerStatus) resource.Quantity {
	// If no storage spec is defined in the autoscaling status we return the default volume size.
	storage := volume.DefaultPersistentVolumeSize.DeepCopy()
	// Always adjust to the min value specified by the user in the limits.
	if autoscalingSpec.IsStorageDefined() {
		storage = autoscalingSpec.StorageRange.Min
	}
	// If a storage value is stored in the status then reuse it.
	if currentResourcesInStatus, exists := currentAutoscalingStatus.CurrentResourcesForPolicy(autoscalingSpec.Name); exists && currentResourcesInStatus.HasRequest(corev1.ResourceStorage) {
		storageInStatus := currentResourcesInStatus.GetRequest(corev1.ResourceStorage)
		if storageInStatus.Cmp(storage) > 0 {
			storage = storageInStatus
		}
	}
	return storage
}

var usableDiskPercent = 0.95

// adjustRequiredStorage adjust the required capacity from Elasticsearch to account for the filesystem reserved space.
// In the worst case we consider that Elasticsearch is only able to use 95% of the persistent volume capacity.
func adjustRequiredStorage(v *client.AutoscalingCapacity) *client.AutoscalingCapacity {
	adjustedStorage := resource.NewQuantity(
		int64(math.Ceil(float64(v.Value())/usableDiskPercent)),
		resource.DecimalSI,
	)
	return &client.AutoscalingCapacity{
		Quantity: *adjustedStorage,
	}
}

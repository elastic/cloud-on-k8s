// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package sset

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	appsv1 "k8s.io/api/apps/v1"
)

// GetReplicas returns the replicas configured for this StatefulSet, or 0 if nil.
func GetReplicas(statefulSet appsv1.StatefulSet) int32 {
	if statefulSet.Spec.Replicas != nil {
		return *statefulSet.Spec.Replicas
	}
	return 0
}

// GetPartition returns the updateStrategy.Partition index, or falls back to the number of replicas if not set.
func GetPartition(statefulSet appsv1.StatefulSet) int32 {
	rollingUpdate := statefulSet.Spec.UpdateStrategy.RollingUpdate
	if rollingUpdate != nil && rollingUpdate.Partition != nil {
		return *rollingUpdate.Partition
	}
	if statefulSet.Spec.Replicas != nil {
		return *statefulSet.Spec.Replicas
	}
	return 0
}

// GetESVersion returns the ES version from the StatefulSet labels.
func GetESVersion(statefulSet appsv1.StatefulSet) (*version.Version, error) {
	return label.ExtractVersion(statefulSet.Spec.Template.Labels)
}

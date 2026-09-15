// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package label

import (
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
)

const (
	// KibanaNameLabelName used to represent a Kibana in k8s resources
	KibanaNameLabelName = "kibana.k8s.elastic.co/name"

	// KibanaNamespaceLabelName used to represent a Kibana in k8s resources
	KibanaNamespaceLabelName = "kibana.k8s.elastic.co/namespace"

	// KibanaVersionLabelName used to propagate Kibana version from the spec to the pods
	KibanaVersionLabelName = "kibana.k8s.elastic.co/version"

	// Type represents the Kibana type
	Type = "kibana"

	// RoleLabelPrefix is the prefix for Kibana role labels applied to pods when background task
	// isolation is enabled. These labels follow the serverless kibana-controller convention.
	RoleLabelPrefix = "kibana.k8s.elastic.co/role-"

	// UIRoleLabelName is the label applied to UI pool pods when background task isolation is enabled.
	UIRoleLabelName = RoleLabelPrefix + "ui"

	// BackgroundTasksRoleLabelName is the label applied to background tasks pool pods.
	BackgroundTasksRoleLabelName = RoleLabelPrefix + "background_tasks"

	// RoleLabelValue is the value used for role labels.
	RoleLabelValue = "true"

	// NodeRolesConfigKey is the Kibana configuration key that selects which roles a process runs.
	// ECK manages this key when background task isolation is enabled; users must not set it.
	NodeRolesConfigKey = "node.roles"
)

// Role describes a Kibana node role and the Kubernetes label that identifies its pods.
// Role labels are only applied when background task isolation is enabled (spec.backgroundTasks is set);
// existing single-pool Kibana resources are never relabeled.
type Role struct {
	// Name is the Kibana node.roles value (e.g. "ui" or "background_tasks").
	Name string
	// LabelName is the Kubernetes pod label key for this role (e.g. UIRoleLabelName).
	LabelName string
}

var (
	// UIRole describes the Kibana UI pool (node.roles: ["ui"]).
	UIRole = Role{
		Name:      "ui",
		LabelName: UIRoleLabelName,
	}

	// BackgroundTasksRole describes the Kibana background tasks pool (node.roles: ["background_tasks"]).
	BackgroundTasksRole = Role{
		Name:      "background_tasks",
		LabelName: BackgroundTasksRoleLabelName,
	}
)

// NewLabels constructs a new set of labels from Kibana definition.
func NewLabels(kb types.NamespacedName) map[string]string {
	return map[string]string{
		KibanaNameLabelName:    kb.Name,
		commonv1.TypeLabelName: Type,
	}
}

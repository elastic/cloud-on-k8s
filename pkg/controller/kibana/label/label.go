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

	// RoleLabelName is the label key applied to pods when background task isolation is enabled.
	// Its value distinguishes the UI/primary pool ("prime") from the background tasks pool
	// ("background_tasks").
	RoleLabelName = "kibana.k8s.elastic.co/role"

	// RolePrimeValue is the label value for the UI / primary pool (node.roles: ["ui"]).
	RolePrimeValue = "prime"

	// RoleBackgroundTasksValue is the label value for the background tasks pool
	// (node.roles: ["background_tasks"]).
	RoleBackgroundTasksValue = "background_tasks"

	// NodeRolesConfigKey is the Kibana configuration key that selects which roles a process runs.
	// ECK manages this key when background task isolation is enabled; users must not set it.
	NodeRolesConfigKey = "node.roles"
)

// Role describes a Kibana node role and its Kubernetes label value.
// Role labels are only applied when background task isolation is enabled (spec.backgroundTasks is set);
// single-pool Kibana resources carry no role label.
type Role struct {
	// Name is the Kibana node.roles value injected via the NODE_ROLES env var
	// (e.g. "ui" or "background_tasks").
	Name string
	// LabelValue is the value written to the kibana.k8s.elastic.co/role label
	// (e.g. "prime" or "background_tasks"). Empty for the single-pool (no-split) case.
	LabelValue string
}

var (
	// UIRole describes the Kibana UI pool: NODE_ROLES=["ui"], role label value "prime".
	UIRole = Role{
		Name:       "ui",
		LabelValue: RolePrimeValue,
	}

	// BackgroundTasksRole describes the Kibana background tasks pool:
	// NODE_ROLES=["background_tasks"], role label value "background_tasks".
	BackgroundTasksRole = Role{
		Name:       "background_tasks",
		LabelValue: RoleBackgroundTasksValue,
	}
)

// NewLabels constructs a new set of labels from Kibana definition.
func NewLabels(kb types.NamespacedName) map[string]string {
	return map[string]string{
		KibanaNameLabelName:    kb.Name,
		commonv1.TypeLabelName: Type,
	}
}

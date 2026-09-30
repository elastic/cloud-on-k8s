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

	// RoleLabelName is the label key applied to pods.
	// Its value distinguishes the UI/primary pool ("primary") from the background tasks pool
	// ("background_tasks").
	RoleLabelName = "kibana.k8s.elastic.co/role"

	// RolePrimaryValue is the label value for the UI / primary pool (node.roles: ["ui"]).
	RolePrimaryValue = "primary"

	// RoleBackgroundTasksValue is the label value for the background tasks pool
	// (node.roles: ["background_tasks"]).
	RoleBackgroundTasksValue = "background_tasks"

	// NodeRolesConfigKey is the Kibana configuration key that selects which roles a process runs.
	// ECK manages this key when background task isolation is enabled; users must not set it.
	NodeRolesConfigKey = "node.roles"

	// NodeRolesEnvVar is the Kibana container environment variable that overrides node.roles.
	// ECK injects this when background task isolation is active (spec.backgroundTasks is set).
	NodeRolesEnvVar = "NODE_ROLES"
)

// Role describes a Kibana node role and its Kubernetes label value.
// The role label is applied to all pools. Single-pool Kibana always carries LabelValue "primary";
// in split mode, UIRole carries "primary" and BackgroundTasksRole carries "background_tasks".
type Role struct {
	// Name is the Kibana node.roles value injected via the NODE_ROLES env var
	// (e.g. "ui" or "background_tasks"). Empty for the single-pool case (no NODE_ROLES injection).
	Name string
	// LabelValue is the value written to the kibana.k8s.elastic.co/role label
	// (e.g. "primary" or "background_tasks").
	LabelValue string
}

var (
	// UIRole describes the Kibana UI pool: NODE_ROLES=["ui"], role label value "primary".
	UIRole = Role{
		Name:       "ui",
		LabelValue: RolePrimaryValue,
	}

	// BackgroundTasksRole describes the Kibana background tasks pool:
	// NODE_ROLES=["background_tasks"], role label value "background_tasks".
	BackgroundTasksRole = Role{
		Name:       "background_tasks",
		LabelValue: RoleBackgroundTasksValue,
	}

	// SinglePoolRole is the synthetic role used when background task isolation is disabled.
	// LabelValue "primary" ensures the selector label is present for upgrade-path detection.
	// Name "" suppresses NODE_ROLES injection — the pod runs all Kibana roles.
	SinglePoolRole = Role{
		Name:       "",
		LabelValue: RolePrimaryValue,
	}
)

func (r Role) IsUI() bool {
	return r == UIRole
}

func (r Role) IsBackgroundTasks() bool {
	return r == BackgroundTasksRole
}

func (r Role) IsSinglePool() bool {
	return r == SinglePoolRole
}

func (r Role) IsPrimary() bool {
	return r == SinglePoolRole || r == UIRole
}

// NewLabels constructs a new set of labels from Kibana definition.
func NewLabels(kb types.NamespacedName) map[string]string {
	return map[string]string{
		KibanaNameLabelName:    kb.Name,
		commonv1.TypeLabelName: Type,
	}
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana/label"
)

// GetIdentityLabels will return the common Elastic assigned labels for the Kibana.
func (k *Kibana) GetIdentityLabels() map[string]string {
	return map[string]string{
		commonv1.TypeLabelName:    "kibana",
		label.KibanaNameLabelName: k.Name,
	}
}

func (k *Kibana) GetPoolIdentityLabels(role label.Role) map[string]string {
	labels := k.GetIdentityLabels()
	labels[label.RoleLabelName] = label.RolePrimeValue

	if k.BackgroundTasksEnabled() && role.LabelValue != "" {
		labels[label.RoleLabelName] = role.LabelValue
	}
	return labels
}

// ActiveRoles returns the list of pools that the controller must reconcile for this Kibana.
// When BackgroundTasks is not set, a single synthetic "all-roles" pool is returned whose
// LabelValue is empty (no role label applied) and whose Name is empty (single-pool behavior).
// When BackgroundTasks is set, both UIRole and BackgroundTasksRole are returned.
func (k *Kibana) ActiveRoles() []label.Role {
	if !k.BackgroundTasksEnabled() {
		// Single all-roles pool: no role label, no NODE_ROLES env var.
		return []label.Role{{Name: "", LabelValue: ""}}
	}
	return []label.Role{label.UIRole, label.BackgroundTasksRole}
}

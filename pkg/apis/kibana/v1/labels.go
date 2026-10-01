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
	labels[label.RoleLabelName] = label.RolePrimaryValue

	if k.BackgroundTasksEnabled() && role.LabelValue != "" {
		labels[label.RoleLabelName] = role.LabelValue
	}
	return labels
}

// ActiveRoles returns the list of pools the controller must reconcile for this Kibana.
// Without split: SinglePoolRole — role=primary selector label, no NODE_ROLES injection.
// With split: UIRole and BackgroundTasksRole, each becoming its own Deployment.
func (k *Kibana) ActiveRoles() []label.Role {
	if !k.BackgroundTasksEnabled() {
		return []label.Role{label.SinglePoolRole}
	}
	return []label.Role{label.UIRole, label.BackgroundTasksRole}
}

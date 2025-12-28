// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/packageregistry/label"
)

// GetIdentityLabels will return the common Elastic assigned labels for the Elastic Package Registry.
func (m *PackageRegistry) GetIdentityLabels() map[string]string {
	return map[string]string{
		commonv1.TypeLabelName: label.Type,
		label.NameLabelName:    m.Name,
	}
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
)

// EnterpriseLicenseLabelName is a label pointing to the name of the source enterprise license.
const EnterpriseLicenseLabelName = "license.k8s.elastic.co/name"

// NewLicenseSelector is a list selector to filter by a label containing the license name.
func NewLicenseSelector(license types.NamespacedName) labels.Selector {
	return labels.Set(map[string]string{
		EnterpriseLicenseLabelName: license.Name,
	}).AsSelector()
}

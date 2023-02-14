// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package maps

import (
	emsv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/maps/v1alpha1"
)

const (
	// NameLabelName used to represent a MapsServer in k8s resources
	NameLabelName = "maps.k8s.elastic.co/name"

	// versionLabelName used to propagate MapsServer version from the spec to the pods
	versionLabelName = "maps.k8s.elastic.co/version"

	// Type represents the MapsServer type
	Type = "maps"
)

func versionLabels(ems emsv1alpha1.ElasticMapsServer) map[string]string {
	return map[string]string{
		versionLabelName: ems.Spec.Version,
	}
}

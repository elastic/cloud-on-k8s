// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package maps

import (
	emsv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/maps/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
)

const (
	// NameLabelName used to represent a MapsServer in k8s resources
	NameLabelName = "maps.k8s.elastic.co/name"

	// versionLabelName used to propagate MapsServer version from the spec to the pods
	versionLabelName = "maps.k8s.elastic.co/version"

	// Type represents the MapsServer type
	Type = "maps"
)

// labels constructs a new set of labels for a MapsServer pod
func labels(emsName string) map[string]string {
	return map[string]string{
		NameLabelName:        emsName,
		common.TypeLabelName: Type,
	}
}

func versionLabels(ems emsv1alpha1.ElasticMapsServer) map[string]string {
	return map[string]string{
		versionLabelName: ems.Spec.Version,
	}
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
)

const (
	// Type represents the Beat type.
	TypeLabelValue = "beat"

	// NameLabelName is used to represent a Beat in k8s resources.
	NameLabelName = "beat.k8s.elastic.co/name"
)

func NewLabels(beat beatv1beta1.Beat) map[string]string {
	return map[string]string{
		common.TypeLabelName: TypeLabelValue,
		NameLabelName:        beat.Name,
	}
}

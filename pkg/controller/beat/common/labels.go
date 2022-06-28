// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	beatv1beta1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/beat/v1beta1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/labels"
)

const (
	// Type represents the Beat type.
	TypeLabelValue = "beat"

	// NameLabelName is used to represent a Beat in k8s resources.
	NameLabelName = "beat.k8s.elastic.co/name"
)

func NewLabels(beat beatv1beta1.Beat) map[string]string {
	return map[string]string{
		labels.TypeLabelName: TypeLabelValue,
		NameLabelName:        beat.Name,
	}
}

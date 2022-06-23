// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
)

func Test_IsGKE(t *testing.T) {
	tests := []struct {
		version string
		isGKE   bool
	}{
		{version: "1.21.6-gke.1503", isGKE: true},
		{version: "1.22.3-gke-4242", isGKE: true},
		{version: "1.21.6", isGKE: false},
		{version: "1.18.16-eks-7737de", isGKE: false},
	}

	ctx = Ctx()
	for _, tt := range tests {
		isGKE := IsGKE(version.MustParse(tt.version))
		if tt.isGKE != isGKE {
			t.Errorf(`version: %s, isGKE() = %v, want %v`, tt.version, isGKE, tt.isGKE)
		}
	}
}

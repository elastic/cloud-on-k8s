// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/validation"
)

func TestNodeLabels_IsAllowed(t *testing.T) {
	type args struct {
		nodeLabel string
	}
	tests := []struct {
		name              string
		exposedNodeLabels []string
		args              args
		want              bool
		wantErr           bool
	}{
		{
			name:              "Cannot parse label",
			exposedNodeLabels: []string{"*"},
			args: args{
				nodeLabel: "kubernetes.io/zone",
			},
			wantErr: true,
		},
		{
			name:              "Matching topology label",
			exposedNodeLabels: []string{"topology.kubernetes.io/*", "failure-domain.beta.kubernetes.io/*"},
			args: args{
				nodeLabel: "topology.kubernetes.io/zone",
			},
			want: true,
		},
		{
			name:              "Matching topology label 2",
			exposedNodeLabels: []string{"topology.kubernetes.io/*", "failure-domain.beta.kubernetes.io/*"},
			args: args{
				nodeLabel: "failure-domain.beta.kubernetes.io/region",
			},
			want: true,
		},
		{
			name:              "Not matching topology label",
			exposedNodeLabels: []string{"topology.kubernetes.io/*", "failure-domain.beta.kubernetes.io/*"},
			args: args{
				nodeLabel: "kubernetes.io/zone",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeLabels, err := validation.NewExposedNodeLabels(tt.exposedNodeLabels)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err, "no error expected when building validation.NodeLabels")
				if got := nodeLabels.IsAllowed(tt.args.nodeLabel); got != tt.want {
					t.Errorf("NodeLabels.IsAllowed() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

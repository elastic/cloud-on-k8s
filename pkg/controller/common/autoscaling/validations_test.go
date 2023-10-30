// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoscaling

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/google/go-cmp/cmp"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1alpha1"
)

var (
	defaultAutoscalingResources = v1alpha1.AutoscalingResources{
		NodeCountRange: v1alpha1.CountRange{
			Min: 1,
			Max: 2,
		},
		CPURange: &v1alpha1.QuantityRange{
			Min: resource.MustParse("1"),
			Max: resource.MustParse("2"),
		},
		MemoryRange: &v1alpha1.QuantityRange{
			Min: resource.MustParse("2Gi"),
			Max: resource.MustParse("4Gi"),
		},
	}
)

func TestValidateAutoscalingPolicies(t *testing.T) {
	type args struct {
		autoscalingSpecPath SpecPathBuilder
		autoscalingPolicies v1alpha1.AutoscalingPolicySpecs
	}
	tests := []struct {
		name string
		args args
		want field.ErrorList
	}{
		{
			name: "ML policy with roles [ml, remote_cluster_client] succeeds",
			args: args{
				autoscalingSpecPath: func(index int, child string, more ...string) *field.Path {
					return field.NewPath("spec").
						Child("policies").
						Index(index).
						Child(child, more...)
				},
				autoscalingPolicies: v1alpha1.AutoscalingPolicySpecs{
					{
						NamedAutoscalingPolicy: v1alpha1.NamedAutoscalingPolicy{
							Name: "data",
							AutoscalingPolicy: v1alpha1.AutoscalingPolicy{
								Roles:    []string{"data", "remote_cluster_client"},
								Deciders: nil,
							},
						},
						AutoscalingResources: defaultAutoscalingResources,
					},
					{
						NamedAutoscalingPolicy: v1alpha1.NamedAutoscalingPolicy{
							Name: "ml",
							AutoscalingPolicy: v1alpha1.AutoscalingPolicy{
								Roles:    []string{"ml", "remote_cluster_client"},
								Deciders: nil,
							},
						},
						AutoscalingResources: defaultAutoscalingResources,
					},
				},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidateAutoscalingPolicies(tt.args.autoscalingSpecPath, tt.args.autoscalingPolicies); !cmp.Equal(got, tt.want) {
				t.Errorf("ValidateAutoscalingPolicies() = diff: %s", cmp.Diff(got, tt.want))
			}
		})
	}
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1

import (
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

func TestAutoscalingSpec_GetMLNodesSettings(t *testing.T) {
	type fields struct {
		AutoscalingPolicySpecs AutoscalingPolicySpecs
	}
	tests := []struct {
		name          string
		fields        fields
		wantNodes     int32
		wantMaxMemory string
	}{
		{
			name: "happy path",
			fields: fields{
				AutoscalingPolicySpecs: AutoscalingPolicySpecs{
					{
						NamedAutoscalingPolicy: NamedAutoscalingPolicy{Name: "ml-policy", AutoscalingPolicy: AutoscalingPolicy{Roles: []string{"ml"}}},
						AutoscalingResources: AutoscalingResources{
							Memory:    &QuantityRange{Min: resource.MustParse("1Gi"), Max: resource.MustParse("8Gi")},
							NodeCount: CountRange{Min: 0, Max: 7},
						},
					},
				},
			},
			wantMaxMemory: "8589934592b",
			wantNodes:     7,
		},
		{
			name: "no dedicated ml tier", // not supported at the time this test is written, but we still want to ensure we return correct values
			fields: fields{
				AutoscalingPolicySpecs: AutoscalingPolicySpecs{
					{
						NamedAutoscalingPolicy: NamedAutoscalingPolicy{Name: "ml-policy", AutoscalingPolicy: AutoscalingPolicy{Roles: []string{"data,ml"}}},
						AutoscalingResources: AutoscalingResources{
							Memory:    &QuantityRange{Min: resource.MustParse("1Gi"), Max: resource.MustParse("8Gi")},
							NodeCount: CountRange{Min: 0, Max: 7},
						},
					},
					{
						NamedAutoscalingPolicy: NamedAutoscalingPolicy{Name: "ml-policy2", AutoscalingPolicy: AutoscalingPolicy{Roles: []string{"ml"}}},
						AutoscalingResources: AutoscalingResources{
							Memory:    &QuantityRange{Min: resource.MustParse("1Gi"), Max: resource.MustParse("16Gi")},
							NodeCount: CountRange{Min: 0, Max: 4},
						},
					},
				},
			},
			wantMaxMemory: "17179869184b",
			wantNodes:     11,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			as := AutoscalingSpec{AutoscalingPolicySpecs: tt.fields.AutoscalingPolicySpecs}
			gotNodes, gotMaxMemory := as.GetMLNodesSettings()
			if gotNodes != tt.wantNodes {
				t.Errorf("AutoscalingSpec.GetMLNodesSettings() gotNodes = %v, want %v", gotNodes, tt.wantNodes)
			}
			if gotMaxMemory != tt.wantMaxMemory {
				t.Errorf("AutoscalingSpec.GetMLNodesSettings() gotMaxMemory = %v, want %v", gotMaxMemory, tt.wantMaxMemory)
			}
		})
	}
}

func TestAutoscalingSpec_findByRoles(t *testing.T) {
	type fields struct {
		AutoscalingPolicySpecs AutoscalingPolicySpecs
	}
	type args struct {
		roles []string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   *AutoscalingPolicySpec
	}{
		{
			name: "Managed by an autoscaling policy",
			fields: fields{
				AutoscalingPolicySpecs: AutoscalingPolicySpecs{
					AutoscalingPolicySpec{
						NamedAutoscalingPolicy: NamedAutoscalingPolicy{
							Name: "ml_only",
							AutoscalingPolicy: AutoscalingPolicy{
								Roles: []string{"ml"},
							},
						},
					}},
			},
			args: args{roles: []string{"ml"}},
			want: &AutoscalingPolicySpec{
				NamedAutoscalingPolicy: NamedAutoscalingPolicy{
					Name: "ml_only",
					AutoscalingPolicy: AutoscalingPolicy{
						Roles: []string{"ml"},
					},
				},
			},
		},
		{
			name: "Not managed by an autoscaling policy",
			fields: fields{
				AutoscalingPolicySpecs: AutoscalingPolicySpecs{
					AutoscalingPolicySpec{
						NamedAutoscalingPolicy: NamedAutoscalingPolicy{
							Name: "ml_only",
							AutoscalingPolicy: AutoscalingPolicy{
								Roles: []string{"ml"},
							},
						},
					}},
			},
			args: args{roles: []string{"master"}},
			want: nil,
		},
		{
			name: "Not managed by an autoscaling policy",
			fields: fields{
				AutoscalingPolicySpecs: AutoscalingPolicySpecs{
					AutoscalingPolicySpec{
						NamedAutoscalingPolicy: NamedAutoscalingPolicy{
							Name: "ml_only",
							AutoscalingPolicy: AutoscalingPolicy{
								Roles: []string{"ml"},
							},
						},
					}},
			},
			args: args{roles: []string{"ml", "data"}},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			as := AutoscalingSpec{
				AutoscalingPolicySpecs: tt.fields.AutoscalingPolicySpecs,
			}
			got := as.findByRoles(tt.args.roles)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AutoscalingSpec.findByRoles() = %v, want %v", got, tt.want)
			}
		})
	}
}

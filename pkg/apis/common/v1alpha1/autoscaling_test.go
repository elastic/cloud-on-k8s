// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	"reflect"
	"testing"
)

func TestAutoscalingPolicySpecs_findByRoles(t *testing.T) {
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
			got := tt.fields.AutoscalingPolicySpecs.FindByRoles(tt.args.roles)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AutoscalingAnnotation.findByRoles() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1alpha1"
)

var logTest = logf.Log.WithName("autoscaling-test")

func Test_newStatusBuilder(t *testing.T) {
	type args struct {
		autoscalingPolicies v1alpha1.AutoscalingPolicySpecs
	}
	tests := []struct {
		name string
		args args
		want []v1alpha1.AutoscalingPolicyStatus
	}{
		{
			name: "Initialize new status with overlapping policies ignores remote_cluster_client role",
			args: args{
				autoscalingPolicies: []v1alpha1.AutoscalingPolicySpec{
					{
						NamedAutoscalingPolicy: v1alpha1.NamedAutoscalingPolicy{Name: "policy1", AutoscalingPolicy: v1alpha1.AutoscalingPolicy{Roles: []string{"role1", "role2", "remote_cluster_client"}}},
					},
					{
						NamedAutoscalingPolicy: v1alpha1.NamedAutoscalingPolicy{Name: "policy2", AutoscalingPolicy: v1alpha1.AutoscalingPolicy{Roles: []string{"role2", "role3", "role5", "remote_cluster_client"}}},
					},
					{
						NamedAutoscalingPolicy: v1alpha1.NamedAutoscalingPolicy{Name: "policy3", AutoscalingPolicy: v1alpha1.AutoscalingPolicy{Roles: []string{"role4", "role2", "role3", "remote_cluster_client"}}},
					},
				},
			},
			want: []v1alpha1.AutoscalingPolicyStatus{
				{
					Name: "policy1",
					PolicyStates: []v1alpha1.PolicyState{{
						Type: "OverlappingPolicies",
						Messages: []string{
							"role role2 is declared in autoscaling policies policy1,policy2,policy3",
						},
					}},
				},
				{
					Name: "policy2",
					PolicyStates: []v1alpha1.PolicyState{{
						Type: "OverlappingPolicies",
						Messages: []string{
							"role role2 is declared in autoscaling policies policy1,policy2,policy3",
							"role role3 is declared in autoscaling policies policy2,policy3",
						},
					}},
				},
				{
					Name: "policy3",
					PolicyStates: []v1alpha1.PolicyState{{
						Type: "OverlappingPolicies",
						Messages: []string{
							"role role2 is declared in autoscaling policies policy1,policy2,policy3",
							"role role3 is declared in autoscaling policies policy2,policy3",
						},
					}},
				},
			},
		},
		{
			name: "No overlapping policies",
			args: args{
				autoscalingPolicies: []v1alpha1.AutoscalingPolicySpec{
					{
						NamedAutoscalingPolicy: v1alpha1.NamedAutoscalingPolicy{Name: "policy1", AutoscalingPolicy: v1alpha1.AutoscalingPolicy{Roles: []string{"role1", "role2"}}},
					},
					{
						NamedAutoscalingPolicy: v1alpha1.NamedAutoscalingPolicy{Name: "policy2", AutoscalingPolicy: v1alpha1.AutoscalingPolicy{Roles: []string{"role3"}}},
					},
					{
						NamedAutoscalingPolicy: v1alpha1.NamedAutoscalingPolicy{Name: "policy3", AutoscalingPolicy: v1alpha1.AutoscalingPolicy{Roles: []string{"role4"}}},
					},
				},
			},
			want: []v1alpha1.AutoscalingPolicyStatus{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			autoscalingStatus := newStatusBuilder(logTest, tt.args.autoscalingPolicies).Build()
			assert.ElementsMatch(t, autoscalingStatus.AutoscalingPolicyStatuses, tt.want)
		})
	}
}

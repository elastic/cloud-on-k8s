// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/autoscaling/elasticsearch/status"
	"github.com/stretchr/testify/assert"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var logTest = logf.Log.WithName("autoscaling-test")

func Test_newStatusBuilder(t *testing.T) {
	type args struct {
		autoscalingSpec esv1.AutoscalingSpec
	}
	tests := []struct {
		name string
		args args
		want []status.AutoscalingPolicyStatus
	}{
		{
			name: "Initialize new status with overlapping policies",
			args: args{
				autoscalingSpec: esv1.AutoscalingSpec{
					AutoscalingPolicySpecs: []esv1.AutoscalingPolicySpec{
						{
							NamedAutoscalingPolicy: esv1.NamedAutoscalingPolicy{Name: "policy1", AutoscalingPolicy: esv1.AutoscalingPolicy{Roles: []string{"role1", "role2"}}},
						},
						{
							NamedAutoscalingPolicy: esv1.NamedAutoscalingPolicy{Name: "policy2", AutoscalingPolicy: esv1.AutoscalingPolicy{Roles: []string{"role2", "role3", "role5"}}},
						},
						{
							NamedAutoscalingPolicy: esv1.NamedAutoscalingPolicy{Name: "policy3", AutoscalingPolicy: esv1.AutoscalingPolicy{Roles: []string{"role4", "role2", "role3"}}},
						},
					},
				},
			},
			want: []status.AutoscalingPolicyStatus{
				{
					Name: "policy1",
					PolicyStates: []status.PolicyState{{
						Type: "OverlappingPolicies",
						Messages: []string{
							"role role2 is declared in autoscaling policies policy1,policy2,policy3",
						},
					}},
				},
				{
					Name: "policy2",
					PolicyStates: []status.PolicyState{{
						Type: "OverlappingPolicies",
						Messages: []string{
							"role role2 is declared in autoscaling policies policy1,policy2,policy3",
							"role role3 is declared in autoscaling policies policy2,policy3",
						},
					}},
				},
				{
					Name: "policy3",
					PolicyStates: []status.PolicyState{{
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
				autoscalingSpec: esv1.AutoscalingSpec{
					AutoscalingPolicySpecs: []esv1.AutoscalingPolicySpec{
						{
							NamedAutoscalingPolicy: esv1.NamedAutoscalingPolicy{Name: "policy1", AutoscalingPolicy: esv1.AutoscalingPolicy{Roles: []string{"role1", "role2"}}},
						},
						{
							NamedAutoscalingPolicy: esv1.NamedAutoscalingPolicy{Name: "policy2", AutoscalingPolicy: esv1.AutoscalingPolicy{Roles: []string{"role3"}}},
						},
						{
							NamedAutoscalingPolicy: esv1.NamedAutoscalingPolicy{Name: "policy3", AutoscalingPolicy: esv1.AutoscalingPolicy{Roles: []string{"role4"}}},
						},
					},
				},
			},
			want: []status.AutoscalingPolicyStatus{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			autoscalingStatus := newStatusBuilder(logTest, tt.args.autoscalingSpec).Build()
			assert.ElementsMatch(t, autoscalingStatus.AutoscalingPolicyStatuses, tt.want)
		})
	}
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import (
	"testing"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/stretchr/testify/assert"
)

func Test_checkSpec(t *testing.T) {
	tests := []struct {
		name    string
		beat    Agent
		wantErr bool
	}{
		{
			name: "deployment absent, dset present",
			beat: Agent{
				Spec: AgentSpec{
					DaemonSet: &DaemonSetSpec{},
				},
			},
			wantErr: false,
		},
		{
			name: "deployment present, dset absent",
			beat: Agent{
				Spec: AgentSpec{
					Deployment: &DeploymentSpec{},
				},
			},
			wantErr: false,
		},
		{
			name: "neither present",
			beat: Agent{
				Spec: AgentSpec{},
			},
			wantErr: true,
		},
		{
			name: "both present",
			beat: Agent{
				Spec: AgentSpec{
					Deployment: &DeploymentSpec{},
					DaemonSet:  &DaemonSetSpec{},
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := checkSpec(&tc.beat)
			assert.Equal(t, tc.wantErr, len(got) > 0)
		})
	}
}

func Test_checkAtMostOneDefaultESRef(t *testing.T) {
	type args struct {
		b *Agent
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "no ref: OK",
			args: args{
				b: &Agent{},
			},
			wantErr: false,
		},
		{
			name: "one default ref: OK",
			args: args{
				b: &Agent{
					Spec: AgentSpec{
						ElasticsearchRefs: []Output{
							{
								ObjectSelector: commonv1.ObjectSelector{Name: "bla", Namespace: "blub"},
								OutputName:     "default",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "one default ref among others: OK",
			args: args{
				b: &Agent{
					Spec: AgentSpec{
						ElasticsearchRefs: []Output{
							{
								ObjectSelector: commonv1.ObjectSelector{Name: "bla", Namespace: "blub"},
								OutputName:     "default",
							},
							{
								ObjectSelector: commonv1.ObjectSelector{Name: "bla", Namespace: "blub"},
								OutputName:     "bla",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "multiple default refs: NOK",
			args: args{
				b: &Agent{
					Spec: AgentSpec{
						ElasticsearchRefs: []Output{
							{
								ObjectSelector: commonv1.ObjectSelector{Name: "bla", Namespace: "blub"},
								OutputName:     "default",
							},
							{
								ObjectSelector: commonv1.ObjectSelector{Name: "bla", Namespace: "blub"},
								OutputName:     "default",
							},
						},
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkAtMostOneDefaultESRef(tt.args.b)
			assert.Equal(t, tt.wantErr, len(got) > 0)

		})
	}
}

func Test_checkESRefsNamed(t *testing.T) {
	type args struct {
		b *Agent
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "no ref: OK",
			args: args{
				b: &Agent{},
			},
			wantErr: false,
		},
		{
			name: "one unnamed ref: OK",
			args: args{
				b: &Agent{
					Spec: AgentSpec{
						ElasticsearchRefs: []Output{
							{
								ObjectSelector: commonv1.ObjectSelector{Name: "bla", Namespace: "blub"},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "multiple named refs: OK",
			args: args{
				b: &Agent{
					Spec: AgentSpec{
						ElasticsearchRefs: []Output{
							{
								ObjectSelector: commonv1.ObjectSelector{Name: "bla", Namespace: "blub"},
								OutputName:     "bla",
							},
							{
								ObjectSelector: commonv1.ObjectSelector{Name: "bla", Namespace: "blub"},
								OutputName:     "blub",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "unamed within multiple: NOK",
			args: args{
				b: &Agent{
					Spec: AgentSpec{
						ElasticsearchRefs: []Output{
							{
								ObjectSelector: commonv1.ObjectSelector{Name: "bla", Namespace: "blub"},
								OutputName:     "",
							},
							{
								ObjectSelector: commonv1.ObjectSelector{Name: "bla", Namespace: "blub"},
								OutputName:     "default",
							},
						},
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkESRefsNamed(tt.args.b)
			assert.Equal(t, tt.wantErr, len(got) > 0)
		})
	}
}

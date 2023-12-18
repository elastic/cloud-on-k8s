// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
)

func Test_checkSupportedVersion(t *testing.T) {
	for _, tt := range []struct {
		name    string
		mode    AgentMode
		version string
		wantErr bool
	}{
		{
			name:    "no fleet, below min supported: NOK",
			mode:    AgentStandaloneMode,
			version: "7.9.2",
			wantErr: true,
		},
		{
			name:    "no fleet, within supported: OK",
			mode:    AgentStandaloneMode,
			version: "7.10.0",
			wantErr: false,
		},
		{
			name:    "fleet, below min supported: NOK",
			mode:    AgentFleetMode,
			version: "7.13.2",
			wantErr: true,
		},
		{
			name:    "fleet, within supported: OK",
			mode:    AgentFleetMode,
			version: "7.14.0-SNAPSHOT",
			wantErr: false,
		},
		{
			name:    "fleet, within supported: OK",
			mode:    AgentFleetMode,
			version: "7.14.0",
			wantErr: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a := Agent{
				Spec: AgentSpec{
					Mode:    tt.mode,
					Version: tt.version,
				},
			}
			got := checkSupportedVersion(&a)
			assert.Equal(t, tt.wantErr, len(got) > 0)
		})
	}
}

func Test_checkPolicyID(t *testing.T) {
	expectedError := field.ErrorList{
		&field.Error{
			Type:     field.ErrorTypeRequired,
			Field:    "spec.policyID",
			BadValue: "",
			Detail:   "Agent policyID is mandatory",
		}}
	tests := []struct {
		name    string
		beat    Agent
		wantErr field.ErrorList
	}{
		{
			name: "no policyID required for 8.x",
			beat: Agent{
				Spec: AgentSpec{
					Version: "8.5.99",
				},
			},
			wantErr: nil,
		},
		{
			name: "policyID required for 9.x",
			beat: Agent{
				Spec: AgentSpec{
					Version: "9.0.0",
				},
			},
			wantErr: expectedError,
		},
		{
			name: "policyID required for 9.0.0-SNAPSHOT",
			beat: Agent{
				Spec: AgentSpec{
					Version: "9.0.0-SNAPSHOT",
				},
			},
			wantErr: expectedError,
		},
		{
			name: "policyID set for 9.x",
			beat: Agent{
				Spec: AgentSpec{
					Version:  "9.0.0",
					PolicyID: "foo",
				},
			},
			wantErr: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := checkPolicyID(&tc.beat)
			assert.Equal(t, tc.wantErr, got)
		})
	}
}

func Test_checkAtMostOneDeploymentOption(t *testing.T) {
	type args struct {
		a *Agent
	}
	tests := []struct {
		name string
		args args
		want field.ErrorList
	}{
		{
			name: "deployment option",
			args: args{
				a: &Agent{Spec: AgentSpec{
					Deployment: &DeploymentSpec{},
				}},
			},
		},
		{
			name: "daemonset option",
			args: args{
				a: &Agent{Spec: AgentSpec{
					DaemonSet: &DaemonSetSpec{},
				}},
			},
		},
		{
			name: "statefulset option",
			args: args{
				a: &Agent{Spec: AgentSpec{
					StatefulSet: &StatefulSetSpec{},
				}},
			},
		},
		{
			name: "statefulset and deployment options",
			args: args{
				a: &Agent{Spec: AgentSpec{
					Deployment:  &DeploymentSpec{},
					StatefulSet: &StatefulSetSpec{},
				}},
			},
			want: field.ErrorList{
				field.Forbidden(field.NewPath("spec.deployment"), "Specify at most one of [deployment, statefulSet]"),
				field.Forbidden(field.NewPath("spec.statefulSet"), "Specify at most one of [deployment, statefulSet]"),
			},
		},
		{
			name: "deployment and daemonset options",
			args: args{
				a: &Agent{Spec: AgentSpec{
					Deployment: &DeploymentSpec{},
					DaemonSet:  &DaemonSetSpec{},
				}},
			},
			want: field.ErrorList{
				field.Forbidden(field.NewPath("spec.daemonSet"), "Specify at most one of [daemonSet, deployment]"),
				field.Forbidden(field.NewPath("spec.deployment"), "Specify at most one of [daemonSet, deployment]"),
			},
		},
		{
			name: "daemonset and statefulset options",
			args: args{
				a: &Agent{Spec: AgentSpec{
					DaemonSet:   &DaemonSetSpec{},
					StatefulSet: &StatefulSetSpec{},
				}},
			},
			want: field.ErrorList{
				field.Forbidden(field.NewPath("spec.daemonSet"), "Specify at most one of [daemonSet, statefulSet]"),
				field.Forbidden(field.NewPath("spec.statefulSet"), "Specify at most one of [daemonSet, statefulSet]"),
			},
		},
		{
			name: "deployment, daemonset, and statefulset options",
			args: args{
				a: &Agent{Spec: AgentSpec{
					Deployment:  &DeploymentSpec{},
					DaemonSet:   &DaemonSetSpec{},
					StatefulSet: &StatefulSetSpec{},
				}},
			},
			want: field.ErrorList{
				field.Forbidden(field.NewPath("spec.daemonSet"), "Specify at most one of [daemonSet, deployment, statefulSet]"),
				field.Forbidden(field.NewPath("spec.deployment"), "Specify at most one of [daemonSet, deployment, statefulSet]"),
				field.Forbidden(field.NewPath("spec.statefulSet"), "Specify at most one of [daemonSet, deployment, statefulSet]"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkAtMostOneDeploymentOption(tt.args.a); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("checkAtMostOneDeploymentOption() = \n%v, \nwant \n%v", got, tt.want)
			}
		})
	}
}

func Test_checkSpec(t *testing.T) {
	tests := []struct {
		name    string
		beat    Agent
		wantErr bool
	}{
		{
			name: "deployment absent, statefulSet absent, daemonSet present",
			beat: Agent{
				Spec: AgentSpec{
					DaemonSet: &DaemonSetSpec{},
				},
			},
			wantErr: false,
		},
		{
			name: "deployment present, statefulSet absent, daemonSet absent",
			beat: Agent{
				Spec: AgentSpec{
					Deployment: &DeploymentSpec{},
				},
			},
			wantErr: false,
		},
		{
			name: "deployment absent, statefulSet present, daemonSet absent",
			beat: Agent{
				Spec: AgentSpec{
					StatefulSet: &StatefulSetSpec{},
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
			name: "both daemonSet and deployment present",
			beat: Agent{
				Spec: AgentSpec{
					Deployment: &DeploymentSpec{},
					DaemonSet:  &DaemonSetSpec{},
				},
			},
			wantErr: true,
		},
		{
			name: "both daemonSet and statefulSet present",
			beat: Agent{
				Spec: AgentSpec{
					DaemonSet:   &DaemonSetSpec{},
					StatefulSet: &StatefulSetSpec{},
				},
			},
			wantErr: true,
		},
		{
			name: "both statefulSet and deployment present",
			beat: Agent{
				Spec: AgentSpec{
					Deployment:  &DeploymentSpec{},
					StatefulSet: &StatefulSetSpec{},
				},
			},
			wantErr: true,
		},
		{
			name: "all statefulSet, daemonSet and deployment present",
			beat: Agent{
				Spec: AgentSpec{
					Deployment:  &DeploymentSpec{},
					StatefulSet: &StatefulSetSpec{},
					DaemonSet:   &DaemonSetSpec{},
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

func Test_checkEmptyConfigForFleetMode(t *testing.T) {
	for _, tt := range []struct {
		name    string
		a       *Agent
		wantErr bool
	}{
		{
			name: "no config: OK",
			a: &Agent{
				Spec: AgentSpec{
					Mode: AgentFleetMode,
				},
			},
			wantErr: false,
		},
		{
			name: "config: NOK",
			a: &Agent{
				Spec: AgentSpec{
					Mode:   AgentFleetMode,
					Config: &commonv1.Config{},
				},
			},
			wantErr: true,
		},
		{
			name: "configref: NOK",
			a: &Agent{
				Spec: AgentSpec{
					Mode:      AgentFleetMode,
					ConfigRef: &commonv1.ConfigSource{},
				},
			},
			wantErr: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := checkEmptyConfigForFleetMode(tt.a)
			assert.Equal(t, tt.wantErr, len(got) > 0)
		})
	}
}

func Test_checkFleetServerOnlyInFleetMode(t *testing.T) {
	for _, tt := range []struct {
		name    string
		a       *Agent
		wantErr bool
	}{
		{
			name: "fleet server not enabled: OK",
			a: &Agent{
				Spec: AgentSpec{
					Mode:               AgentStandaloneMode,
					FleetServerEnabled: false,
				},
			},
			wantErr: false,
		},
		{
			name: "fleet server enabled: NOK",
			a: &Agent{
				Spec: AgentSpec{
					Mode:               AgentStandaloneMode,
					FleetServerEnabled: true,
				},
			},
			wantErr: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := checkFleetServerOnlyInFleetMode(tt.a)
			assert.Equal(t, tt.wantErr, len(got) > 0)
		})
	}
}

func Test_checkFleetServerOrFleetServerRef(t *testing.T) {
	for _, tt := range []struct {
		name    string
		a       *Agent
		wantErr bool
	}{
		{
			name: "fleet server without fleet server ref: OK",
			a: &Agent{
				Spec: AgentSpec{
					FleetServerEnabled: true,
				},
			},
			wantErr: false,
		},
		{
			name: "fleet server with fleet server ref: NOK",
			a: &Agent{
				Spec: AgentSpec{
					FleetServerEnabled: true,
					FleetServerRef:     commonv1.ObjectSelector{Name: "name"},
				},
			},
			wantErr: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := checkFleetServerOrFleetServerRef(tt.a)
			assert.Equal(t, tt.wantErr, len(got) > 0)
		})
	}
}

func Test_checkHTTPConfigOnlyForFleetServer(t *testing.T) {
	for _, tt := range []struct {
		name    string
		a       *Agent
		wantErr bool
	}{
		{
			name: "fleet server with service configuration: OK",
			a: &Agent{
				Spec: AgentSpec{
					FleetServerEnabled: true,
					HTTP:               commonv1.HTTPConfig{},
				},
			},
			wantErr: false,
		},
		{
			name: "no fleet server with service configuration: NOK",
			a: &Agent{
				Spec: AgentSpec{
					FleetServerEnabled: false,
					HTTP: commonv1.HTTPConfig{TLS: commonv1.TLSOptions{
						Certificate: commonv1.SecretRef{
							SecretName: "name",
						},
					}},
				},
			},
			wantErr: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := checkHTTPConfigOnlyForFleetServer(tt.a)
			assert.Equal(t, tt.wantErr, len(got) > 0)
		})
	}
}

func Test_checkReferenceSetForMode(t *testing.T) {
	for _, tt := range []struct {
		name    string
		a       *Agent
		wantErr bool
	}{
		{
			name: "standalone mode - no fleet server ref, no kibana ref: OK",
			a: &Agent{
				Spec: AgentSpec{
					Mode: AgentStandaloneMode,
				},
			},
			wantErr: false,
		},
		{
			name: "standalone mode - fleet server ref, no kibana ref: NOK",
			a: &Agent{
				Spec: AgentSpec{
					Mode:           AgentStandaloneMode,
					FleetServerRef: commonv1.ObjectSelector{Name: "name"},
				},
			},
			wantErr: true,
		},
		{
			name: "standalone mode - no fleet server ref, kibana ref: NOK",
			a: &Agent{
				Spec: AgentSpec{
					Mode:      AgentStandaloneMode,
					KibanaRef: commonv1.ObjectSelector{Name: "name"},
				},
			},
			wantErr: true,
		},
		{
			name: "standalone mode - fleet server ref, kibana ref: NOK",
			a: &Agent{
				Spec: AgentSpec{
					Mode:           AgentStandaloneMode,
					FleetServerRef: commonv1.ObjectSelector{Name: "name"},
					KibanaRef:      commonv1.ObjectSelector{Name: "name"},
				},
			},
			wantErr: true,
		},
		{
			name: "fleet mode - fleet server, elasticsearch ref: OK",
			a: &Agent{
				Spec: AgentSpec{
					Mode:               AgentFleetMode,
					FleetServerEnabled: true,
					ElasticsearchRefs: []Output{{
						ObjectSelector: commonv1.ObjectSelector{Name: "name"},
						OutputName:     "name",
					}},
				},
			},
			wantErr: false,
		},
		{
			name: "fleet mode - no fleet server, elasticsearch ref: NOK",
			a: &Agent{
				Spec: AgentSpec{
					Mode:               AgentFleetMode,
					FleetServerEnabled: false,
					ElasticsearchRefs: []Output{{
						ObjectSelector: commonv1.ObjectSelector{Name: "name"},
						OutputName:     "name",
					}},
				},
			},
			wantErr: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := checkReferenceSetForMode(tt.a)
			assert.Equal(t, tt.wantErr, len(got) > 0)
		})
	}
}

func Test_checkAssociations(t *testing.T) {
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
			name: "mix secret named and named refs: OK",
			args: args{
				b: &Agent{
					Spec: AgentSpec{
						ElasticsearchRefs: []Output{
							{
								ObjectSelector: commonv1.ObjectSelector{SecretName: "bla"},
							},
							{
								ObjectSelector: commonv1.ObjectSelector{Name: "bla", Namespace: "blub"},
							},
						},
						KibanaRef:      commonv1.ObjectSelector{Name: "bli", Namespace: "blub"},
						FleetServerRef: commonv1.ObjectSelector{SecretName: "ble"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "secret named ref with a name: NOK",
			args: args{
				b: &Agent{
					Spec: AgentSpec{
						ElasticsearchRefs: []Output{
							{
								ObjectSelector: commonv1.ObjectSelector{SecretName: "bla", Name: "bla"},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "no name or secret name with namespace: NOK",
			args: args{
				b: &Agent{
					Spec: AgentSpec{
						KibanaRef: commonv1.ObjectSelector{Namespace: "blub"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "no name or secret name with serviceName: NOK",
			args: args{
				b: &Agent{
					Spec: AgentSpec{
						FleetServerRef: commonv1.ObjectSelector{ServiceName: "ble"},
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkAssociations(tt.args.b)
			assert.Equal(t, tt.wantErr, len(got) > 0)
		})
	}
}

func Test_checkNoDowngrade(t *testing.T) {
	type args struct {
		prev *Agent
		curr *Agent
	}
	tests := []struct {
		name string
		args args
		want field.ErrorList
	}{
		{
			name: "No downgrade",
			args: args{
				prev: &Agent{Spec: AgentSpec{Version: "7.17.0"}},
				curr: &Agent{Spec: AgentSpec{Version: "8.2.0"}},
			},
			want: nil,
		},
		{
			name: "Downgrade NOK",
			args: args{
				prev: &Agent{Spec: AgentSpec{Version: "8.2.0"}},
				curr: &Agent{Spec: AgentSpec{Version: "8.1.0"}},
			},
			want: field.ErrorList{&field.Error{Type: field.ErrorTypeForbidden, Field: "spec.version", BadValue: "", Detail: "Version downgrades are not supported"}},
		},
		{
			name: "Downgrade with override OK",
			args: args{
				prev: &Agent{Spec: AgentSpec{Version: "8.2.0"}},
				curr: &Agent{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
					commonv1.DisableDowngradeValidationAnnotation: "true",
				}}, Spec: AgentSpec{Version: "8.1.0"}},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, checkNoDowngrade(tt.args.prev, tt.args.curr), "checkNoDowngrade(%v, %v)", tt.args.prev, tt.args.curr)
		})
	}
}

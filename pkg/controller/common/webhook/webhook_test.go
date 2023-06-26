// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package webhook

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/agent/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/set"
)

func asJSON(obj interface{}) []byte {
	data, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}
	return data
}

func Test_validatingWebhook_Handle(t *testing.T) {
	type fields struct {
		managedNamespaces set.StringSet
		validator         admission.Validator
	}
	tests := []struct {
		name   string
		fields fields
		req    admission.Request
		want   admission.Response
	}{
		{
			name: "create properly validates valid agent, and returns allowed.",
			fields: fields{
				set.Make("elastic"),
				&agentv1alpha1.Agent{},
			},
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: asJSON(&agentv1alpha1.Agent{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "testAgent",
								Namespace: "elastic",
								Labels: map[string]string{
									"test": "label1",
								},
							},
							Spec: agentv1alpha1.AgentSpec{
								Version:    "7.10.0",
								Deployment: &agentv1alpha1.DeploymentSpec{},
								PolicyID:   "a-policy",
							},
						}),
					},
				},
			},
			want: admission.Allowed(""),
		},
		{
			name: "no policy id when agent running in standalone mode should not return a warning",
			fields: fields{
				set.Make("elastic"),
				&agentv1alpha1.Agent{},
			},
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: asJSON(&agentv1alpha1.Agent{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "testAgent",
								Namespace: "elastic",
								Labels: map[string]string{
									"test": "label1",
								},
							},
							Spec: agentv1alpha1.AgentSpec{
								Version:    "7.10.0",
								Deployment: &agentv1alpha1.DeploymentSpec{},
							},
						}),
					},
				},
			},
			want: admission.Allowed(""),
		},
		{
			name: "no policy id is allowed when agent running in fleet mode but it should return a warning",
			fields: fields{
				set.Make("elastic"),
				&agentv1alpha1.Agent{},
			},
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: asJSON(&agentv1alpha1.Agent{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "testAgent",
								Namespace: "elastic",
								Labels: map[string]string{
									"test": "label1",
								},
							},
							Spec: agentv1alpha1.AgentSpec{
								Version:    "7.14.0",
								Deployment: &agentv1alpha1.DeploymentSpec{},
								Mode:       agentv1alpha1.AgentFleetMode,
							},
						}),
					},
				},
			},
			want: admission.Allowed("").WithWarnings("Agent elastic/testAgent: spec.PolicyID is empty, spec.PolicyID will become mandatory in a future release"),
		},
		{
			name: "create agent is denied because of invalid version, and returns denied.",
			fields: fields{
				set.Make("elastic"),
				&agentv1alpha1.Agent{},
			},
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: asJSON(&agentv1alpha1.Agent{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "testAgent",
								Namespace: "elastic",
								Labels: map[string]string{
									"test": "label1",
								},
							},
							Spec: agentv1alpha1.AgentSpec{
								Version:    "0.10.0",
								Deployment: &agentv1alpha1.DeploymentSpec{},
								PolicyID:   "a-policy",
							},
						}),
					},
				},
			},
			want: admission.Denied(`Agent.agent.k8s.elastic.co "testAgent" is invalid: spec.version: Invalid value: "0.10.0": Unsupported version: version 0.10.0 is lower than the lowest supported version of 7.10.0`),
		},
		{
			name: "delete agent is always allowed",
			fields: fields{
				set.Make("elastic"),
				&agentv1alpha1.Agent{},
			},
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Delete,
					Object: runtime.RawExtension{
						Raw: asJSON(&agentv1alpha1.Agent{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "testAgent",
								Namespace: "elastic",
								Labels: map[string]string{
									"test": "label1",
								},
							},
							Spec: agentv1alpha1.AgentSpec{
								Version:    "7.10.0",
								Deployment: &agentv1alpha1.DeploymentSpec{},
								PolicyID:   "a-policy",
							},
						}),
					},
				},
			},
			want: admission.Allowed(""),
		},
		{
			name: "request from un-managed namespace is ignored, and just accepted",
			fields: fields{
				set.Make("elastic"),
				&agentv1alpha1.Agent{},
			},
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Delete,
					Object: runtime.RawExtension{
						Raw: asJSON(&agentv1alpha1.Agent{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "testAgent",
								Namespace: "unmanaged",
								Labels: map[string]string{
									"test": "label1",
								},
							},
							Spec: agentv1alpha1.AgentSpec{
								Version:    "7.10.0",
								Deployment: &agentv1alpha1.DeploymentSpec{},
							},
						}),
					},
				},
			},
			want: admission.Allowed(""),
		},
		{
			name: "update agent is allowed when label is updated",
			fields: fields{
				set.Make("elastic"),
				&agentv1alpha1.Agent{},
			},
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					OldObject: runtime.RawExtension{
						Raw: asJSON(&agentv1alpha1.Agent{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "testAgent",
								Namespace: "elastic",
								Labels: map[string]string{
									"test": "label1",
								},
							},
							Spec: agentv1alpha1.AgentSpec{
								Version:    "7.10.0",
								Deployment: &agentv1alpha1.DeploymentSpec{},
								PolicyID:   "a-policy",
							},
						}),
					},
					Object: runtime.RawExtension{
						Raw: asJSON(&agentv1alpha1.Agent{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "testAgent",
								Namespace: "elastic",
								Labels: map[string]string{
									"test": "label2",
								},
							},
							Spec: agentv1alpha1.AgentSpec{
								Version:    "7.10.0",
								Deployment: &agentv1alpha1.DeploymentSpec{},
								PolicyID:   "a-policy",
							},
						}),
					},
				},
			},
			want: admission.Allowed(""),
		},
		{
			name: "update agent is denied when version downgrade is attempted",
			fields: fields{
				set.Make("elastic"),
				&agentv1alpha1.Agent{},
			},
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					OldObject: runtime.RawExtension{
						Raw: asJSON(&agentv1alpha1.Agent{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "testAgent",
								Namespace: "elastic",
								Labels: map[string]string{
									"test": "label1",
								},
							},
							Spec: agentv1alpha1.AgentSpec{
								Version:    "7.10.1",
								Deployment: &agentv1alpha1.DeploymentSpec{},
								PolicyID:   "a-policy",
							},
						}),
					},
					Object: runtime.RawExtension{
						Raw: asJSON(&agentv1alpha1.Agent{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "testAgent",
								Namespace: "elastic",
								Labels: map[string]string{
									"test": "label1",
								},
							},
							Spec: agentv1alpha1.AgentSpec{
								Version:    "7.10.0",
								Deployment: &agentv1alpha1.DeploymentSpec{},
								PolicyID:   "a-policy",
							},
						}),
					},
				},
			},
			want: admission.Denied(`Agent.agent.k8s.elastic.co "testAgent" is invalid: spec.version: Forbidden: Version downgrades are not supported`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			decoder := admission.NewDecoder(k8s.Scheme())
			v := &validatingWebhook{
				decoder:           decoder,
				managedNamespaces: tt.fields.managedNamespaces,
				validator:         tt.fields.validator,
			}
			if got := v.Handle(ctx, tt.req); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("validatingWebhook.Handle() = %v, want %v", got, tt.want)
			}
		})
	}
}

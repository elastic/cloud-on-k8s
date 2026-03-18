// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	commonwebhook "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/webhook"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func asJSON(obj any) []byte {
	data, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}
	return data
}

func Test_validator_Handle(t *testing.T) {
	type fields struct {
		client               k8s.Client
		validateStorageClass bool
	}
	tests := []struct {
		name         string
		fields       fields
		req          admission.Request
		wantAllowed  bool
		wantMessage  string
		wantWarnings []string
	}{
		{
			name: "accept valid creation",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Object: runtime.RawExtension{
					Raw: asJSON(&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"},
						Spec:       esv1.ElasticsearchSpec{Version: "8.9.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
					}),
				}},
			},
			wantAllowed: true,
		},
		{
			name: "request from un-managed namespace is ignored, and just accepted",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Object: runtime.RawExtension{
					Raw: asJSON(&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{Namespace: "unmanaged", Name: "name"},
						Spec:       esv1.ElasticsearchSpec{Version: "8.9.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
					}),
				}},
			},
			wantAllowed: true,
		},
		{
			name: "reject invalid creation (no version provided)",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Object: runtime.RawExtension{
					Raw: asJSON(&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"},
						Spec:       esv1.ElasticsearchSpec{NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
					}),
				}},
			},
			wantAllowed: false,
			wantMessage: parseVersionErrMsg,
		},
		{
			name: "accept valid update (count++)",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Update,
				OldObject: runtime.RawExtension{
					Raw: asJSON(&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"},
						Spec:       esv1.ElasticsearchSpec{Version: "8.9.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
					}),
				},
				Object: runtime.RawExtension{
					Raw: asJSON(&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"},
						Spec:       esv1.ElasticsearchSpec{Version: "8.9.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 4}}},
					}),
				},
			}},
			wantAllowed: true,
		},
		{
			name: "reject invalid update (version downgrade))",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Update,
				OldObject: runtime.RawExtension{
					Raw: asJSON(&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"},
						Spec:       esv1.ElasticsearchSpec{Version: "8.9.1", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
					}),
				},
				Object: runtime.RawExtension{
					Raw: asJSON(&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"},
						Spec:       esv1.ElasticsearchSpec{Version: "8.9.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
					}),
				},
			}},
			wantAllowed: false,
			wantMessage: noDowngradesMsg,
		},
		{
			name: "reject invalid update (from 8.9.0 to 9.0.0))",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Update,
				OldObject: runtime.RawExtension{
					Raw: asJSON(&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"},
						Spec:       esv1.ElasticsearchSpec{Version: "8.9.1", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
					}),
				},
				Object: runtime.RawExtension{
					Raw: asJSON(&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"},
						Spec:       esv1.ElasticsearchSpec{Version: "9.0.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
					}),
				},
			}},
			wantAllowed: false,
			wantMessage: unsupportedUpgradeMsg,
		},
		{
			name: "accept valid update (from 8.18.0 to 9.0.0))",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Update,
				OldObject: runtime.RawExtension{
					Raw: asJSON(&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"},
						Spec:       esv1.ElasticsearchSpec{Version: "8.18.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
					}),
				},
				Object: runtime.RawExtension{
					Raw: asJSON(&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"},
						Spec:       esv1.ElasticsearchSpec{Version: "9.0.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
					}),
				},
			}},
			wantAllowed: true,
		},
		{
			name: "accept creation with zone-awareness DoesNotExist affinity (warning at reconcile time, not admission error)",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			args: args{
				req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: asJSON(&esv1.Elasticsearch{
							ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"},
							Spec: esv1.ElasticsearchSpec{
								Version: "8.9.0",
								NodeSets: []esv1.NodeSet{
									{
										Name:          "set1",
										Count:         3,
										ZoneAwareness: &esv1.ZoneAwareness{},
										PodTemplate: corev1.PodTemplateSpec{
											Spec: corev1.PodSpec{
												Affinity: &corev1.Affinity{
													NodeAffinity: &corev1.NodeAffinity{
														RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
															NodeSelectorTerms: []corev1.NodeSelectorTerm{
																{
																	MatchExpressions: []corev1.NodeSelectorRequirement{
																		{
																			Key:      esv1.DefaultZoneAwarenessTopologyKey,
																			Operator: corev1.NodeSelectorOpDoesNotExist,
																		},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						}),
					}},
				},
			},
			want: admission.Allowed(""),
		},
		{
			name: "reject creation when zone-awareness zones conflict with In affinity values",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			args: args{
				req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: asJSON(&esv1.Elasticsearch{
							ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"},
							Spec: esv1.ElasticsearchSpec{
								Version: "8.9.0",
								NodeSets: []esv1.NodeSet{
									{
										Name:          "set1",
										Count:         3,
										ZoneAwareness: &esv1.ZoneAwareness{Zones: []string{"us-east-1a", "us-east-1b"}},
										PodTemplate: corev1.PodTemplateSpec{
											Spec: corev1.PodSpec{
												Affinity: &corev1.Affinity{
													NodeAffinity: &corev1.NodeAffinity{
														RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
															NodeSelectorTerms: []corev1.NodeSelectorTerm{
																{
																	MatchExpressions: []corev1.NodeSelectorRequirement{
																		{
																			Key:      esv1.DefaultZoneAwarenessTopologyKey,
																			Operator: corev1.NodeSelectorOpIn,
																			Values:   []string{"us-east-1c"},
																		},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						}),
					}},
				},
			},
			want: admission.Denied(zoneAwarenessAffinityInNoIntersectionMsg),
		},
		{
			name: "accept valid creation with warnings due to deprecated version",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Object: runtime.RawExtension{
					Raw: asJSON(&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"},
						Spec:       esv1.ElasticsearchSpec{Version: "7.9.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
					}),
				}},
			},
			wantAllowed:  true,
			wantWarnings: []string{"Version 7.9.0 is EOL and support for it will be removed in a future release of the ECK operator"},
		},
		{
			name: "reject downgrade on deprecated version but still return warnings",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Update,
				OldObject: runtime.RawExtension{
					Raw: asJSON(&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"},
						Spec:       esv1.ElasticsearchSpec{Version: "7.10.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
					}),
				},
				Object: runtime.RawExtension{
					Raw: asJSON(&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"},
						Spec:       esv1.ElasticsearchSpec{Version: "7.9.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
					}),
				},
			}},
			wantAllowed:  false,
			wantMessage:  noDowngradesMsg,
			wantWarnings: []string{"Version 7.9.0 is EOL and support for it will be removed in a future release of the ECK operator"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inner := &validator{
				client:               tt.fields.client,
				validateStorageClass: tt.fields.validateStorageClass,
				licenseChecker:       license.MockLicenseChecker{},
			}
			v := commonwebhook.NewResourceValidator[*esv1.Elasticsearch](nil, []string{"ns"}, inner)
			wh := admission.WithValidator[*esv1.Elasticsearch](k8s.Scheme(), v)
			got := wh.Handle(context.Background(), tt.req)
			require.Equal(t, tt.wantAllowed, got.Allowed)
			if !got.Allowed {
				require.Contains(t, got.Result.Message, tt.wantMessage)
			}
			if len(tt.wantWarnings) > 0 {
				require.Equal(t, tt.wantWarnings, got.Warnings)
			}
		})
	}
}

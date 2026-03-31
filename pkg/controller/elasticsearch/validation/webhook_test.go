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

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/set"
)

func asJSON(obj any) []byte {
	data, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}
	return data
}

func esPod(namespace, clusterName, name, triggerValue string) *corev1.Pod {
	p := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				label.ClusterNameLabelName: clusterName,
			},
		},
	}
	if triggerValue != "" {
		p.Annotations = map[string]string{
			esv1.RestartTriggerAnnotation: triggerValue,
		}
	}
	return p
}

func Test_validatingWebhook_Handle(t *testing.T) {
	decoder := admission.NewDecoder(k8s.Scheme())
	type fields struct {
		client               k8s.Client
		validateStorageClass bool
	}
	type args struct {
		req admission.Request
	}
	tests := []struct {
		name         string
		fields       fields
		args         args
		want         admission.Response
		wantWarnings []string
	}{
		{
			name: "accept valid creation",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			args: args{
				req: admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Operation: admissionv1.Create,
						Object: runtime.RawExtension{
							Raw: asJSON(&esv1.Elasticsearch{
								ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"},
								Spec:       esv1.ElasticsearchSpec{Version: "8.9.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
							}),
						},
					},
				},
			},
			want: admission.Allowed(""),
		},
		{
			name: "request from un-managed namespace is ignored, and just accepted",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			args: args{
				req: admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Operation: admissionv1.Create,
						Object: runtime.RawExtension{
							Raw: asJSON(&esv1.Elasticsearch{
								ObjectMeta: metav1.ObjectMeta{Namespace: "unmanaged", Name: "name"},
								Spec:       esv1.ElasticsearchSpec{Version: "8.9.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
							}),
						},
					},
				},
			},
			want: admission.Allowed(""),
		},
		{
			name: "reject invalid creation (no version provided)",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			args: args{
				req: admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Operation: admissionv1.Create,
						Object: runtime.RawExtension{
							Raw: asJSON(&esv1.Elasticsearch{
								ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"},
								Spec:       esv1.ElasticsearchSpec{NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
							}),
						},
					},
				},
			},
			want: admission.Denied(parseVersionErrMsg),
		},
		{
			name: "accept valid update (count++)",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			args: args{
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
			},
			want: admission.Allowed(""),
		},
		{
			name: "reject invalid update (version downgrade))",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			args: args{
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
			},
			want: admission.Denied(noDowngradesMsg),
		},
		{
			name: "reject invalid update (from 8.9.0 to 9.0.0))",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			args: args{
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
			},
			want: admission.Denied(unsupportedUpgradeMsg),
		},
		{
			name: "accept valid update (from 8.18.0 to 9.0.0))",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			args: args{
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
			},
			want: admission.Allowed(""),
		},
		{
			name: "accept creation with zone-awareness DoesNotExist affinity (warning at reconcile time, not admission error)",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			args: args{
				req: admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
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
						},
					},
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
				req: admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
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
						},
					},
				},
			},
			want: admission.Denied(zoneAwarenessAffinityInNoIntersectionMsg),
		},
		{
			name: "accept valid creation with warnings due to deprecated version",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			args: args{
				req: admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Operation: admissionv1.Create,
						Object: runtime.RawExtension{
							Raw: asJSON(&esv1.Elasticsearch{
								ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"},
								Spec:       esv1.ElasticsearchSpec{Version: "7.9.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
							}),
						},
					},
				},
			},
			want:         admission.Allowed(""),
			wantWarnings: []string{"Version 7.9.0 is EOL and support for it will be removed in a future release of the ECK operator"},
		},
		{
			name: "update: restart-trigger annotation removed while restart in progress emits warning",
			fields: fields{
				client: k8s.NewFakeClient(
					esPod("ns", "name", "pod-0", "v1"),
					esPod("ns", "name", "pod-1", "old-value"),
				),
			},
			args: args{
				req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					OldObject: runtime.RawExtension{
						Raw: asJSON(&esv1.Elasticsearch{
							ObjectMeta: metav1.ObjectMeta{
								Namespace:   "ns",
								Name:        "name",
								Annotations: map[string]string{esv1.RestartTriggerAnnotation: "v1"},
							},
							Spec: esv1.ElasticsearchSpec{Version: "8.9.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
						}),
					},
					Object: runtime.RawExtension{
						Raw: asJSON(&esv1.Elasticsearch{
							ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"},
							Spec:       esv1.ElasticsearchSpec{Version: "8.9.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
						}),
					},
				}},
			},
			want:         admission.Allowed(""),
			wantWarnings: []string{restartTriggerRemovedWarningMsg},
		},
		{
			name: "update: restart-trigger annotation removed, no restart in progress, no warning",
			fields: fields{
				client: k8s.NewFakeClient(
					esPod("ns", "name", "pod-0", "v1"),
					esPod("ns", "name", "pod-1", "v1"),
				),
			},
			args: args{
				req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					OldObject: runtime.RawExtension{
						Raw: asJSON(&esv1.Elasticsearch{
							ObjectMeta: metav1.ObjectMeta{
								Namespace:   "ns",
								Name:        "name",
								Annotations: map[string]string{esv1.RestartTriggerAnnotation: "v1"},
							},
							Spec: esv1.ElasticsearchSpec{Version: "8.9.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
						}),
					},
					Object: runtime.RawExtension{
						Raw: asJSON(&esv1.Elasticsearch{
							ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"},
							Spec:       esv1.ElasticsearchSpec{Version: "8.9.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
						}),
					},
				}},
			},
			want: admission.Allowed(""),
		},
		{
			name: "update: restart-trigger annotation re-added with value pods already have emits warning",
			fields: fields{
				client: k8s.NewFakeClient(
					esPod("ns", "name", "pod-0", "v1"),
					esPod("ns", "name", "pod-1", "v1"),
				),
			},
			args: args{
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
							ObjectMeta: metav1.ObjectMeta{
								Namespace:   "ns",
								Name:        "name",
								Annotations: map[string]string{esv1.RestartTriggerAnnotation: "v1"},
							},
							Spec: esv1.ElasticsearchSpec{Version: "8.9.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
						}),
					},
				}},
			},
			want:         admission.Allowed(""),
			wantWarnings: []string{restartTriggerUnchangedWarningMsg},
		},
		{
			name: "update: restart-trigger annotation changed to new value, no warning",
			fields: fields{
				client: k8s.NewFakeClient(
					esPod("ns", "name", "pod-0", "v1"),
					esPod("ns", "name", "pod-1", "v1"),
				),
			},
			args: args{
				req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					OldObject: runtime.RawExtension{
						Raw: asJSON(&esv1.Elasticsearch{
							ObjectMeta: metav1.ObjectMeta{
								Namespace:   "ns",
								Name:        "name",
								Annotations: map[string]string{esv1.RestartTriggerAnnotation: "v1"},
							},
							Spec: esv1.ElasticsearchSpec{Version: "8.9.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
						}),
					},
					Object: runtime.RawExtension{
						Raw: asJSON(&esv1.Elasticsearch{
							ObjectMeta: metav1.ObjectMeta{
								Namespace:   "ns",
								Name:        "name",
								Annotations: map[string]string{esv1.RestartTriggerAnnotation: "v2"},
							},
							Spec: esv1.ElasticsearchSpec{Version: "8.9.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
						}),
					},
				}},
			},
			want: admission.Allowed(""),
		},
		{
			name: "update: deprecated version + restart-trigger removal warning are both returned",
			fields: fields{
				client: k8s.NewFakeClient(
					esPod("ns", "name", "pod-0", "v1"),
					esPod("ns", "name", "pod-1", "old-value"),
				),
			},
			args: args{
				req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					OldObject: runtime.RawExtension{
						Raw: asJSON(&esv1.Elasticsearch{
							ObjectMeta: metav1.ObjectMeta{
								Namespace:   "ns",
								Name:        "name",
								Annotations: map[string]string{esv1.RestartTriggerAnnotation: "v1"},
							},
							Spec: esv1.ElasticsearchSpec{Version: "7.9.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
						}),
					},
					Object: runtime.RawExtension{
						Raw: asJSON(&esv1.Elasticsearch{
							ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"},
							Spec:       esv1.ElasticsearchSpec{Version: "7.9.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
						}),
					},
				}},
			},
			want: admission.Allowed(""),
			wantWarnings: []string{
				"Version 7.9.0 is EOL and support for it will be removed in a future release of the ECK operator",
				restartTriggerRemovedWarningMsg,
			},
		},
		{
			name: "restart allocation delay warning on update",
			fields: fields{
				client: k8s.NewFakeClient(
					esPod("ns", "name", "pod-0", ""),
					esPod("ns", "name", "pod-1", ""),
				),
			},
			args: args{
				req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					OldObject: runtime.RawExtension{
						Raw: asJSON(&esv1.Elasticsearch{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "ns",
								Name:      "name",
							},
							Spec: esv1.ElasticsearchSpec{Version: "8.19.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
						}),
					},
					Object: runtime.RawExtension{
						Raw: asJSON(&esv1.Elasticsearch{
							ObjectMeta: metav1.ObjectMeta{
								Namespace:   "ns",
								Name:        "name",
								Annotations: map[string]string{esv1.RestartAllocationDelayAnnotation: "-10s"},
							},
							Spec: esv1.ElasticsearchSpec{Version: "8.19.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
						}),
					},
				}},
			},
			want: admission.Allowed(""),
			wantWarnings: []string{
				"restart-allocation-delay annotation will be ignored due to error: negative restart-allocation-delay annotation: -10s",
			},
		},
		{
			name: "restart allocation delay warning on create",
			fields: fields{
				client: k8s.NewFakeClient(
					esPod("ns", "name", "pod-0", ""),
					esPod("ns", "name", "pod-1", ""),
				),
			},
			args: args{
				req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: asJSON(&esv1.Elasticsearch{
							ObjectMeta: metav1.ObjectMeta{
								Namespace:   "ns",
								Name:        "name",
								Annotations: map[string]string{esv1.RestartAllocationDelayAnnotation: "-10s"},
							},
							Spec: esv1.ElasticsearchSpec{Version: "8.19.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
						}),
					},
				}},
			},
			want: admission.Allowed(""),
			wantWarnings: []string{
				"restart-allocation-delay annotation will be ignored due to error: negative restart-allocation-delay annotation: -10s",
			},
		},
		{
			name: "accept valid creation with warning due to mixed fips nodesets",
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
									{Name: "set1", Count: 1, Config: &commonv1.Config{Data: map[string]any{"xpack.security.fips_mode.enabled": true}}},
									{Name: "set2", Count: 1, Config: &commonv1.Config{Data: map[string]any{"xpack.security.fips_mode.enabled": false}}},
								},
							},
						}),
					},
				}},
			},
			want:         admission.Allowed(""),
			wantWarnings: []string{inconsistentFIPSModeWarningMsg},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wh := &validatingWebhook{
				client:               tt.fields.client,
				decoder:              decoder,
				validateStorageClass: tt.fields.validateStorageClass,
				managedNamespaces:    set.Make("ns"),
			}
			got := wh.Handle(context.Background(), tt.args.req)
			require.Equal(t, tt.want.Allowed, got.Allowed)
			if !got.Allowed {
				require.Contains(t, got.Result.Reason, tt.want.Result.Reason)
			}
			require.Equal(t, tt.wantWarnings, got.Warnings)
		})
	}
}

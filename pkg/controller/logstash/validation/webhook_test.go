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
	"k8s.io/apimachinery/pkg/runtime"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
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

func Test_webhook_Create(t *testing.T) {
	decoder := admission.NewDecoder(k8s.Scheme())
	type fields struct {
		client               k8s.Client
		validateStorageClass bool
	}
	type args struct {
		req admission.Request
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   admission.Response
	}{
		{
			name: "simple-logstash",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			args: args{
				req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: asJSON(
							&v1alpha1.Logstash{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "webhook-test",
									Namespace: "ns",
								},
								Spec: v1alpha1.LogstashSpec{
									Version: "8.12.0",
								},
							},
						),
					}},
				},
			},
			want: admission.Allowed(""),
		},
		{
			name: "invalid-stack-version",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			args: args{
				req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: asJSON(
							&v1alpha1.Logstash{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "webhook-test",
									Namespace: "ns",
								},
								Spec: v1alpha1.LogstashSpec{
									Version: "8.11.1",
								},
							},
						),
					}},
				},
			},
			want: admission.Denied("spec.version: Invalid value: \"8.11.1\": Unsupported version: version 8.11.1 is lower than the lowest supported version of 8.12.0"),
		},
		{
			name: "simple-stackmon-ref",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			args: args{
				req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: asJSON(
							&v1alpha1.Logstash{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "webhook-test",
									Namespace: "ns",
								},
								Spec: v1alpha1.LogstashSpec{
									Version: "8.12.0",
									Monitoring: commonv1.Monitoring{
										Metrics: commonv1.MetricsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "esmonname", Namespace: "esmonns"}}},
									},
								},
							},
						),
					}},
				},
			},
			want: admission.Allowed(""),
		},
		{
			name: "multiple-stackmon-ref",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			args: args{
				req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: asJSON(
							&v1alpha1.Logstash{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "webhook-test",
									Namespace: "ns",
								},
								Spec: v1alpha1.LogstashSpec{
									Version: "8.12.0",
									Monitoring: commonv1.Monitoring{
										Metrics: commonv1.MetricsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{SecretName: "es1monname"}}},
										Logs:    commonv1.LogsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{SecretName: "es2monname"}}},
									},
								},
							},
						),
					}},
				},
			},
			want: admission.Allowed(""),
		},
		{
			name: "invalid-stackmon-ref-with-name",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			args: args{
				req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: asJSON(
							&v1alpha1.Logstash{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "webhook-test",
									Namespace: "ns",
								},
								Spec: v1alpha1.LogstashSpec{
									Version: "8.12.0",
									Monitoring: commonv1.Monitoring{
										Metrics: commonv1.MetricsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{SecretName: "es1monname", Name: "xx"}}},
										Logs:    commonv1.LogsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{SecretName: "es2monname"}}},
									},
								},
							},
						),
					}},
				},
			},
			want: admission.Denied("spec.monitoring.metrics: Forbidden: Invalid association reference: specify name or secretName, not both"),
		},
		{
			name: "invalid-stackmon-ref-with-service-name",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			args: args{
				req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: asJSON(
							&v1alpha1.Logstash{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "webhook-test",
									Namespace: "ns",
								},
								Spec: v1alpha1.LogstashSpec{
									Version: "8.12.0",
									Monitoring: commonv1.Monitoring{
										Metrics: commonv1.MetricsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{SecretName: "es1monname"}}},
										Logs:    commonv1.LogsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{SecretName: "es2monname", ServiceName: "xx"}}},
									},
								},
							},
						),
					}},
				},
			},
			want: admission.Denied("spec.monitoring.logs: Forbidden: Invalid association reference: serviceName or namespace can only be used in combination with name, not with secretName"),
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
				require.Contains(t, got.Result.Message, tt.want.Result.Message)
			}
		})
	}
}

func Test_webhook_Update(t *testing.T) {
	decoder := admission.NewDecoder(k8s.Scheme())
	type fields struct {
		client               k8s.Client
		validateStorageClass bool
	}
	type args struct {
		req admission.Request
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   admission.Response
	}{
		{
			name: "accept version upgrade",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			args: args{
				req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					OldObject: runtime.RawExtension{
						Raw: asJSON(
							&v1alpha1.Logstash{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "webhook-test",
									Namespace: "ns",
								},
								Spec: v1alpha1.LogstashSpec{
									Version: "8.12.0",
								},
							},
						),
					},
					Object: runtime.RawExtension{
						Raw: asJSON(
							&v1alpha1.Logstash{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "webhook-test",
									Namespace: "ns",
								},
								Spec: v1alpha1.LogstashSpec{
									Version: "8.12.1",
								},
							},
						),
					},
				}},
			},
			want: admission.Allowed(""),
		},
		{
			name: "deny version downgrade",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			args: args{
				req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					OldObject: runtime.RawExtension{
						Raw: asJSON(
							&v1alpha1.Logstash{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "webhook-test",
									Namespace: "ns",
								},
								Spec: v1alpha1.LogstashSpec{
									Version: "8.12.1",
								},
							},
						),
					},
					Object: runtime.RawExtension{
						Raw: asJSON(
							&v1alpha1.Logstash{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "webhook-test",
									Namespace: "ns",
								},
								Spec: v1alpha1.LogstashSpec{
									Version: "8.12.0",
								},
							},
						),
					},
				}},
			},
			want: admission.Denied("spec.version: Forbidden: Version downgrades are not supported"),
		},
		{
			name: "allows storage increase",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			args: args{
				req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					OldObject: runtime.RawExtension{
						Raw: asJSON(
							&v1alpha1.Logstash{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "webhook-test",
									Namespace: "ns",
								},
								Spec: v1alpha1.LogstashSpec{
									Version: "8.12.0",
									VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
										{ObjectMeta: metav1.ObjectMeta{
											Name: "test-pq",
										},
											Spec: corev1.PersistentVolumeClaimSpec{
												Resources: corev1.VolumeResourceRequirements{
													Requests: corev1.ResourceList{
														corev1.ResourceStorage: resource.MustParse("1Gi"),
													},
												},
											},
										},
									},
								},
							},
						),
					},
					Object: runtime.RawExtension{
						Raw: asJSON(
							&v1alpha1.Logstash{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "webhook-test",
									Namespace: "ns",
								},
								Spec: v1alpha1.LogstashSpec{
									Version: "8.12.0",
									VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
										{ObjectMeta: metav1.ObjectMeta{
											Name: "test-pq",
										},
											Spec: corev1.PersistentVolumeClaimSpec{
												Resources: corev1.VolumeResourceRequirements{
													Requests: corev1.ResourceList{
														corev1.ResourceStorage: resource.MustParse("2Gi"),
													},
												},
											},
										},
									},
								},
							},
						),
					},
				}},
			},
			want: admission.Allowed(""),
		},
		{
			name: "does not allow storage decrease",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			args: args{
				req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					OldObject: runtime.RawExtension{
						Raw: asJSON(
							&v1alpha1.Logstash{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "webhook-test",
									Namespace: "ns",
								},
								Spec: v1alpha1.LogstashSpec{
									Version: "8.12.0",
									VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
										{ObjectMeta: metav1.ObjectMeta{
											Name: "test-pq",
										},
											Spec: corev1.PersistentVolumeClaimSpec{
												Resources: corev1.VolumeResourceRequirements{
													Requests: corev1.ResourceList{
														corev1.ResourceStorage: resource.MustParse("2Gi"),
													},
												},
											},
										},
									},
								},
							},
						),
					},
					Object: runtime.RawExtension{
						Raw: asJSON(
							&v1alpha1.Logstash{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "webhook-test",
									Namespace: "ns",
								},
								Spec: v1alpha1.LogstashSpec{
									Version: "8.12.0",
									VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
										{ObjectMeta: metav1.ObjectMeta{
											Name: "test-pq",
										},
											Spec: corev1.PersistentVolumeClaimSpec{
												Resources: corev1.VolumeResourceRequirements{
													Requests: corev1.ResourceList{
														corev1.ResourceStorage: resource.MustParse("1Gi"),
													},
												},
											},
										},
									},
								},
							},
						),
					},
				}},
			},
			want: admission.Denied("decreasing storage size is not supported: an attempt was made to decrease storage size for claim test-pq"),
		},
		{
			name: "denies storage increase with default storage class",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			args: args{
				req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					OldObject: runtime.RawExtension{
						Raw: asJSON(
							&v1alpha1.Logstash{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "webhook-test",
									Namespace: "ns",
								},
								Spec: v1alpha1.LogstashSpec{
									Version: "8.12.0",
									VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
										{ObjectMeta: metav1.ObjectMeta{
											Name: "test-pq",
										},
											Spec: corev1.PersistentVolumeClaimSpec{
												Resources: corev1.VolumeResourceRequirements{
													Requests: corev1.ResourceList{
														corev1.ResourceStorage: resource.MustParse("1Gi"),
													},
												},
											},
										},
									},
								},
							},
						),
					},
					Object: runtime.RawExtension{
						Raw: asJSON(
							&v1alpha1.Logstash{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "webhook-test",
									Namespace: "ns",
								},
								Spec: v1alpha1.LogstashSpec{
									Version: "8.12.0",
									VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
										{ObjectMeta: metav1.ObjectMeta{
											Name: "test-pq",
										},
											Spec: corev1.PersistentVolumeClaimSpec{
												AccessModes: []corev1.PersistentVolumeAccessMode{
													corev1.ReadWriteOnce,
												},

												Resources: corev1.VolumeResourceRequirements{
													Requests: corev1.ResourceList{
														corev1.ResourceStorage: resource.MustParse("2Gi"),
													},
												},
											},
										},
									},
								},
							},
						),
					},
				}},
			},
			want: admission.Denied("Volume claim templates can only have storage requests modified"),
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
				require.Equal(t, got.Result.Reason, tt.want.Result.Reason)
				require.Contains(t, got.Result.Message, tt.want.Result.Message)
			}
		})
	}
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"context"
	"testing"

	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/pkg/utils/compare"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestReconcileService(t *testing.T) {
	owner := &kbv1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "owner-obj",
			Namespace: "test",
		},
	}

	existingSvc := mkService()
	client := k8s.WrappedFakeClient(owner, existingSvc)

	expectedSvc := mkService()
	delete(expectedSvc.Labels, "lbl2")
	delete(expectedSvc.Annotations, "ann2")
	expectedSvc.Labels["lbl3"] = "lblval3"
	expectedSvc.Annotations["ann3"] = "annval3"

	wantSvc := mkService()
	wantSvc.Labels["lbl3"] = "lblval3"
	wantSvc.Annotations["ann3"] = "annval3"

	haveSvc, err := ReconcileService(context.Background(), client, expectedSvc, owner)
	require.NoError(t, err)
	comparison.AssertEqual(t, wantSvc, haveSvc)
}

func mkService() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "owner-svc",
			Namespace:   "test",
			Labels:      map[string]string{"lbl1": "lblval1", "lbl2": "lbl2val"},
			Annotations: map[string]string{"ann1": "annval1", "ann2": "annval2"},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"foo": "bar"},
			Ports: []corev1.ServicePort{
				{Name: "https", Port: 443},
			},
		},
	}
}

func Test_needsUpdate(t *testing.T) {
	type args struct {
		expected   corev1.Service
		reconciled corev1.Service
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "Spec changes trigger updates",
			args: args{
				expected: corev1.Service{Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeLoadBalancer,
				}},
				reconciled: corev1.Service{Spec: corev1.ServiceSpec{
					Type:      corev1.ServiceTypeClusterIP,
					ClusterIP: "None",
				}},
			},
			want: true,
		},
		{
			name: "Metadata changes trigger updates",
			args: args{
				expected: corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "kibana-service",
						Labels:      map[string]string{"label1": "newval"},
						Annotations: map[string]string{"annotation1": "annotation1val"},
					},
				},
				reconciled: corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "kibana-service",
						Labels:      map[string]string{"label1": "label1val", "label2": "label2val"},
						Annotations: map[string]string{"annotation1": "annotation1val", "annotation2": "annotation2val"},
					},
				},
			},
			want: true,
		},
		{
			name: "Defaulted, auto-assigned values and additional metadata are ignored", // see Test_applyServerSideValues for more extensive tests
			args: args{
				expected: corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "kibana-service",
						Labels:      map[string]string{"label1": "label1val"},
						Annotations: map[string]string{"annotation1": "annotation1val"},
					},
					Spec: corev1.ServiceSpec{

						Type: corev1.ServiceTypeClusterIP,
					}},
				reconciled: corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "kibana-service",
						Labels:      map[string]string{"label1": "label1val", "label2": "label2val"},
						Annotations: map[string]string{"annotation1": "annotation1val", "annotation2": "annotation2val"},
					},
					Spec: corev1.ServiceSpec{
						Type:      corev1.ServiceTypeClusterIP,
						ClusterIP: "1.2.3.4",
					}},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := needsUpdate(&tt.args.expected, &tt.args.reconciled); got != tt.want {
				t.Errorf("needsUpdate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_needsDelete(t *testing.T) {
	type args struct {
		expected   corev1.Service
		reconciled corev1.Service
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "Needs delete if clusterIP changes (IP)",
			args: args{
				expected: corev1.Service{Spec: corev1.ServiceSpec{
					Type:      corev1.ServiceTypeClusterIP,
					ClusterIP: "4.3.2.1",
				}},
				reconciled: corev1.Service{Spec: corev1.ServiceSpec{
					Type:      corev1.ServiceTypeClusterIP,
					ClusterIP: "1.2.3.4",
				}},
			},
			want: true,
		},
		{
			name: "Needs delete if clusterIP changes (None)",
			args: args{
				expected: corev1.Service{Spec: corev1.ServiceSpec{
					Type:      corev1.ServiceTypeClusterIP,
					ClusterIP: "None",
				}},
				reconciled: corev1.Service{Spec: corev1.ServiceSpec{
					Type:      corev1.ServiceTypeClusterIP,
					ClusterIP: "1.2.3.4",
				}},
			},
			want: true,
		},
		{
			name: "Does not need delete if service type changes",
			args: args{
				expected: corev1.Service{Spec: corev1.ServiceSpec{
					Type:      corev1.ServiceTypeLoadBalancer,
					ClusterIP: "1.2.3.4",
				}},
				reconciled: corev1.Service{Spec: corev1.ServiceSpec{
					Type:      corev1.ServiceTypeClusterIP,
					ClusterIP: "1.2.3.4",
				}},
			},
			want: false,
		},
		{
			name: "Does not need to delete if master assigns IP address",
			args: args{
				expected: corev1.Service{Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeClusterIP,
				}},
				reconciled: corev1.Service{Spec: corev1.ServiceSpec{
					Type:      corev1.ServiceTypeClusterIP,
					ClusterIP: "1.2.3.4",
				}},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := needsRecreate(&tt.args.expected, &tt.args.reconciled); got != tt.want {
				t.Errorf("needsRecreate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_applyServerSideValues(t *testing.T) {
	type args struct {
		expected   corev1.Service
		reconciled corev1.Service
	}
	tests := []struct {
		name string
		args args
		want corev1.Service
	}{
		{
			name: "Reconciled ClusterIP/Type/SessionAffinity is used if expected ClusterIP/Type/SessionAffinity is empty",
			args: args{
				expected: corev1.Service{Spec: corev1.ServiceSpec{}},
				reconciled: corev1.Service{Spec: corev1.ServiceSpec{
					Type:            corev1.ServiceTypeClusterIP,
					ClusterIP:       "1.2.3.4",
					SessionAffinity: corev1.ServiceAffinityClientIP,
				}},
			},
			want: corev1.Service{Spec: corev1.ServiceSpec{
				Type:            corev1.ServiceTypeClusterIP,
				ClusterIP:       "1.2.3.4",
				SessionAffinity: corev1.ServiceAffinityClientIP,
			}},
		},
		{
			name: "Reconciled ClusterIP is not used if the reconciled ClusterIP is not an IP",
			args: args{
				expected: corev1.Service{Spec: corev1.ServiceSpec{}},
				reconciled: corev1.Service{Spec: corev1.ServiceSpec{
					Type:            corev1.ServiceTypeClusterIP,
					ClusterIP:       "None",
					SessionAffinity: corev1.ServiceAffinityClientIP,
				}},
			},
			want: corev1.Service{Spec: corev1.ServiceSpec{
				Type:            corev1.ServiceTypeClusterIP,
				SessionAffinity: corev1.ServiceAffinityClientIP,
			}},
		},
		{
			name: "Reconciled ClusterIP is not used if expected type changes",
			args: args{
				expected: corev1.Service{Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeLoadBalancer,
				}},
				reconciled: corev1.Service{Spec: corev1.ServiceSpec{
					Type:      corev1.ServiceTypeClusterIP,
					ClusterIP: "1.2.3.4",
				}},
			},
			want: corev1.Service{Spec: corev1.ServiceSpec{
				Type: corev1.ServiceTypeLoadBalancer,
			}},
		},
		{
			name: "Reconciled ClusterIP/Type/SessionAffinity is not used if expected ClusterIP/Type/SessionAffinity is set",
			args: args{
				expected: corev1.Service{Spec: corev1.ServiceSpec{
					Type:            corev1.ServiceTypeLoadBalancer,
					ClusterIP:       "4.3.2.1",
					SessionAffinity: corev1.ServiceAffinityNone,
				}},
				reconciled: corev1.Service{Spec: corev1.ServiceSpec{
					Type:            corev1.ServiceTypeClusterIP,
					ClusterIP:       "1.2.3.4",
					SessionAffinity: corev1.ServiceAffinityClientIP,
				}},
			},
			want: corev1.Service{Spec: corev1.ServiceSpec{
				Type:            corev1.ServiceTypeLoadBalancer,
				ClusterIP:       "4.3.2.1",
				SessionAffinity: corev1.ServiceAffinityNone,
			}},
		},
		{
			name: "Reconciled target ports are used",
			args: args{
				expected: corev1.Service{Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: int32(9200)}},
				}},
				reconciled: corev1.Service{Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: int32(9200), TargetPort: intstr.FromInt(9200)}},
				}},
			},
			want: corev1.Service{Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{Port: int32(9200), TargetPort: intstr.FromInt(9200)}},
			}},
		},
		{
			name: "Reconciled target ports are not used if there is not the same number of ports",
			args: args{
				expected: corev1.Service{Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: int32(9200)}, {Port: int32(9300)}},
				}},
				reconciled: corev1.Service{Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: int32(9200), TargetPort: intstr.FromInt(9200)}},
				}},
			},
			want: corev1.Service{Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{Port: int32(9200)}, {Port: int32(9300)}},
			}},
		},
		{
			name: "Reconciled node ports are used",
			args: args{
				expected: corev1.Service{Spec: corev1.ServiceSpec{
					Type:  corev1.ServiceTypeLoadBalancer,
					Ports: []corev1.ServicePort{{Port: int32(9200)}},
				}},
				reconciled: corev1.Service{Spec: corev1.ServiceSpec{
					Type:  corev1.ServiceTypeLoadBalancer,
					Ports: []corev1.ServicePort{{Port: int32(9200), NodePort: int32(33433)}},
				}},
			},
			want: corev1.Service{Spec: corev1.ServiceSpec{
				Type:  corev1.ServiceTypeLoadBalancer,
				Ports: []corev1.ServicePort{{Port: int32(9200), NodePort: int32(33433)}},
			}},
		},
		{
			name: "Reconciled node ports are not used if defined in the expected service",
			args: args{
				expected: corev1.Service{Spec: corev1.ServiceSpec{
					Type:  corev1.ServiceTypeNodePort,
					Ports: []corev1.ServicePort{{Port: int32(9200), NodePort: int32(33111)}},
				}},
				reconciled: corev1.Service{Spec: corev1.ServiceSpec{
					Type:  corev1.ServiceTypeLoadBalancer,
					Ports: []corev1.ServicePort{{Port: int32(9200), NodePort: int32(33222)}},
				}},
			},
			want: corev1.Service{Spec: corev1.ServiceSpec{
				Type:  corev1.ServiceTypeNodePort,
				Ports: []corev1.ServicePort{{Port: int32(9200), NodePort: int32(33111)}},
			}},
		},
		{
			name: "Reconciled node ports are not used if it makes no sense depending the service type",
			args: args{
				expected: corev1.Service{Spec: corev1.ServiceSpec{
					Type:  corev1.ServiceTypeClusterIP,
					Ports: []corev1.ServicePort{{Port: int32(9200)}},
				}},
				reconciled: corev1.Service{Spec: corev1.ServiceSpec{
					Type:  corev1.ServiceTypeLoadBalancer,
					Ports: []corev1.ServicePort{{Port: int32(9200), NodePort: int32(33433)}},
				}},
			},
			want: corev1.Service{Spec: corev1.ServiceSpec{
				Type:  corev1.ServiceTypeClusterIP,
				Ports: []corev1.ServicePort{{Port: int32(9200)}},
			}},
		},
		{
			name: "Reconciled health check node ports are used",
			args: args{
				expected: corev1.Service{Spec: corev1.ServiceSpec{
					Type:                  corev1.ServiceTypeLoadBalancer,
					ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeLocal,
					Ports:                 []corev1.ServicePort{{Port: int32(9200)}},
				}},
				reconciled: corev1.Service{Spec: corev1.ServiceSpec{
					Type:                  corev1.ServiceTypeLoadBalancer,
					ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeLocal,
					HealthCheckNodePort:   32767,
					Ports:                 []corev1.ServicePort{{Port: int32(9200), NodePort: int32(33433)}},
				}},
			},
			want: corev1.Service{Spec: corev1.ServiceSpec{
				Type:                  corev1.ServiceTypeLoadBalancer,
				ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeLocal,
				HealthCheckNodePort:   32767,
				Ports:                 []corev1.ServicePort{{Port: int32(9200), NodePort: int32(33433)}},
			}},
		},
		{
			name: "Annotations and labels are preserved",
			args: args{
				expected: corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "kibana-service",
						Labels:      map[string]string{"label1": "label1val"},
						Annotations: map[string]string{"annotation1": "annotation1val"},
					},
					Spec: corev1.ServiceSpec{
						Type:      corev1.ServiceTypeClusterIP,
						ClusterIP: "1.2.3.4",
					},
				},
				reconciled: corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "kibana-service",
						Labels:      map[string]string{"label1": "label1val", "label2": "label2val"},
						Annotations: map[string]string{"annotation1": "annotation1val", "annotation2": "annotation2val"},
					},
					Spec: corev1.ServiceSpec{
						Type:      corev1.ServiceTypeClusterIP,
						ClusterIP: "1.2.3.4",
					},
				},
			},
			want: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "kibana-service",
					Labels:      map[string]string{"label1": "label1val", "label2": "label2val"},
					Annotations: map[string]string{"annotation1": "annotation1val", "annotation2": "annotation2val"},
				},
				Spec: corev1.ServiceSpec{
					Type:      corev1.ServiceTypeClusterIP,
					ClusterIP: "1.2.3.4",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyServerSideValues(&tt.args.expected, &tt.args.reconciled)
			compare.JSONEqual(t, tt.want, tt.args.expected)
		})
	}
}

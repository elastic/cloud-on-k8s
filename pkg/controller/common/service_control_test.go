// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	kbv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/compare"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func TestReconcileService(t *testing.T) {
	owner := &kbv1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "owner-obj",
			Namespace: "test",
		},
	}

	existingSvc := mkService(owner)
	client := k8s.NewFakeClient(owner, existingSvc)

	expectedSvc := mkService(owner)
	delete(expectedSvc.Labels, "lbl2")
	delete(expectedSvc.Annotations, "ann2")
	expectedSvc.Labels["lbl3"] = "lblval3"
	expectedSvc.Annotations["ann3"] = "annval3"

	wantSvc := mkService(owner)
	wantSvc.Labels["lbl3"] = "lblval3"
	wantSvc.Annotations["ann3"] = "annval3"

	haveSvc, err := ReconcileService(context.Background(), client, expectedSvc, owner)
	require.NoError(t, err)
	comparison.AssertEqual(t, wantSvc, haveSvc)
}

func mkService(owner *kbv1.Kibana) *corev1.Service {
	trueVal := true
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "owner-svc",
			Namespace:   "test",
			Labels:      map[string]string{"lbl1": "lblval1", "lbl2": "lbl2val"},
			Annotations: map[string]string{"ann1": "annval1", "ann2": "annval2"},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "kibana.k8s.elastic.co/v1",
					Kind:               "Kibana",
					Name:               owner.Name,
					Controller:         &trueVal,
					BlockOwnerDeletion: &trueVal,
				},
			},
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
	ipFamilySingleStack := corev1.IPFamilyPolicySingleStack
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
						Type:           corev1.ServiceTypeClusterIP,
						ClusterIP:      "1.2.3.4",
						ClusterIPs:     []string{"1.2.3.4"},
						IPFamilies:     []corev1.IPFamily{corev1.IPv4Protocol},
						IPFamilyPolicy: &ipFamilySingleStack,
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

func Test_needsRecreate(t *testing.T) {
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
			name: "Needs delete if ipFamily changes",
			args: args{
				expected: corev1.Service{Spec: corev1.ServiceSpec{
					Type:       corev1.ServiceTypeClusterIP,
					IPFamilies: []corev1.IPFamily{corev1.IPv4Protocol},
				}},
				reconciled: corev1.Service{Spec: corev1.ServiceSpec{
					Type:       corev1.ServiceTypeClusterIP,
					IPFamilies: []corev1.IPFamily{corev1.IPv6Protocol},
				}},
			},
			want: true,
		},
		{
			name: "Does not need delete if ipFamily is not defined",
			args: args{
				expected: corev1.Service{Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeClusterIP,
				}},
				reconciled: corev1.Service{Spec: corev1.ServiceSpec{
					Type:       corev1.ServiceTypeClusterIP,
					IPFamilies: []corev1.IPFamily{corev1.IPv6Protocol},
				}},
			},
			want: false,
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
		{
			name: "No need to recreate if LoadBalancerClass has not changed",
			args: args{
				expected: corev1.Service{Spec: corev1.ServiceSpec{
					Type:              corev1.ServiceTypeLoadBalancer,
					LoadBalancerClass: ptr.To("my-customer/lb"),
				}},
				reconciled: corev1.Service{Spec: corev1.ServiceSpec{
					Type:              corev1.ServiceTypeLoadBalancer,
					LoadBalancerClass: ptr.To("my-customer/lb"),
				}},
			},
			want: false,
		},
		{
			name: "Needs recreate if LoadBalancerClass is changed",
			args: args{
				expected: corev1.Service{Spec: corev1.ServiceSpec{
					Type:              corev1.ServiceTypeLoadBalancer,
					LoadBalancerClass: ptr.To("my-customer/lb"),
				}},
				reconciled: corev1.Service{Spec: corev1.ServiceSpec{
					Type:              corev1.ServiceTypeLoadBalancer,
					LoadBalancerClass: ptr.To("something/else"),
				}},
			},
			want: true,
		},
		{
			name: "Removing the load balancer class is OK if target type is no longer LoadBalancer",
			args: args{
				expected: corev1.Service{Spec: corev1.ServiceSpec{
					Type:              corev1.ServiceTypeClusterIP,
					LoadBalancerClass: nil,
				}},
				reconciled: corev1.Service{Spec: corev1.ServiceSpec{
					Type:              corev1.ServiceTypeLoadBalancer,
					LoadBalancerClass: ptr.To("something/else"),
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
	pointer := func(policyType corev1.ServiceInternalTrafficPolicyType) *corev1.ServiceInternalTrafficPolicyType {
		return &policyType
	}
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
			name: "Reconciled ClusterIP/ClusterIPs/Type/SessionAffinity is used if expected ClusterIP/Type/SessionAffinity is empty",
			args: args{
				expected: corev1.Service{Spec: corev1.ServiceSpec{}},
				reconciled: corev1.Service{Spec: corev1.ServiceSpec{
					Type:            corev1.ServiceTypeClusterIP,
					ClusterIP:       "1.2.3.4",
					ClusterIPs:      []string{"1.2.3.4"},
					SessionAffinity: corev1.ServiceAffinityClientIP,
				}},
			},
			want: corev1.Service{Spec: corev1.ServiceSpec{
				Type:            corev1.ServiceTypeClusterIP,
				ClusterIP:       "1.2.3.4",
				ClusterIPs:      []string{"1.2.3.4"},
				SessionAffinity: corev1.ServiceAffinityClientIP,
			}},
		},
		{
			name: "Reconciled ClusterIP[s] is also used if the reconciled ClusterIP[s] are not valid IPs",
			args: args{
				expected: corev1.Service{Spec: corev1.ServiceSpec{}},
				reconciled: corev1.Service{Spec: corev1.ServiceSpec{
					Type:            corev1.ServiceTypeClusterIP,
					ClusterIP:       "None",
					ClusterIPs:      []string{"None"},
					SessionAffinity: corev1.ServiceAffinityClientIP,
				}},
			},
			want: corev1.Service{Spec: corev1.ServiceSpec{
				Type:            corev1.ServiceTypeClusterIP,
				ClusterIP:       "None",
				ClusterIPs:      []string{"None"},
				SessionAffinity: corev1.ServiceAffinityClientIP,
			}},
		},
		{
			name: "Reconciled ClusterIP[s] is not used if expected type changes",
			args: args{
				expected: corev1.Service{Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeLoadBalancer,
				}},
				reconciled: corev1.Service{Spec: corev1.ServiceSpec{
					Type:       corev1.ServiceTypeClusterIP,
					ClusterIP:  "1.2.3.4",
					ClusterIPs: []string{"1.2.3.4"},
				}},
			},
			want: corev1.Service{Spec: corev1.ServiceSpec{
				Type: corev1.ServiceTypeLoadBalancer,
			}},
		},
		{
			name: "Reconciled ClusterIP[s]/Type/SessionAffinity is not used if expected ClusterIP[s]/Type/SessionAffinity is set",
			args: args{
				expected: corev1.Service{Spec: corev1.ServiceSpec{
					Type:            corev1.ServiceTypeLoadBalancer,
					ClusterIP:       "4.3.2.1",
					ClusterIPs:      []string{"4.3.2.1"},
					SessionAffinity: corev1.ServiceAffinityNone,
				}},
				reconciled: corev1.Service{Spec: corev1.ServiceSpec{
					Type:            corev1.ServiceTypeClusterIP,
					ClusterIP:       "1.2.3.4",
					ClusterIPs:      []string{"1.2.3.4"},
					SessionAffinity: corev1.ServiceAffinityClientIP,
				}},
			},
			want: corev1.Service{Spec: corev1.ServiceSpec{
				Type:            corev1.ServiceTypeLoadBalancer,
				ClusterIP:       "4.3.2.1",
				ClusterIPs:      []string{"4.3.2.1"},
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
		{
			name: "Reconciled IPFamily is used if empty",
			args: args{
				expected: corev1.Service{Spec: corev1.ServiceSpec{}},
				reconciled: corev1.Service{Spec: corev1.ServiceSpec{
					Type:            corev1.ServiceTypeClusterIP,
					ClusterIP:       "1.2.3.4",
					SessionAffinity: corev1.ServiceAffinityClientIP,
					IPFamilies:      []corev1.IPFamily{corev1.IPv6Protocol},
				}},
			},
			want: corev1.Service{Spec: corev1.ServiceSpec{
				Type:            corev1.ServiceTypeClusterIP,
				ClusterIP:       "1.2.3.4",
				SessionAffinity: corev1.ServiceAffinityClientIP,
				IPFamilies:      []corev1.IPFamily{corev1.IPv6Protocol},
			}},
		},
		{
			name: "Provided IPFamily is used if not empty",
			args: args{
				expected: corev1.Service{Spec: corev1.ServiceSpec{
					IPFamilies: []corev1.IPFamily{corev1.IPv6Protocol},
				}},
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
				IPFamilies:      []corev1.IPFamily{corev1.IPv6Protocol},
			}},
		},
		{
			name: "Reconciled InternalTrafficPolicy/ExternalTrafficPolicy/AllocateLoadBalancerPorts are used if the expected one is empty",
			args: args{
				expected: corev1.Service{Spec: corev1.ServiceSpec{}},
				reconciled: corev1.Service{Spec: corev1.ServiceSpec{
					InternalTrafficPolicy:         pointer(corev1.ServiceInternalTrafficPolicyCluster),
					ExternalTrafficPolicy:         corev1.ServiceExternalTrafficPolicyCluster,
					AllocateLoadBalancerNodePorts: ptr.To(true),
				}},
			},
			want: corev1.Service{Spec: corev1.ServiceSpec{
				InternalTrafficPolicy:         pointer(corev1.ServiceInternalTrafficPolicyCluster),
				ExternalTrafficPolicy:         corev1.ServiceExternalTrafficPolicyCluster,
				AllocateLoadBalancerNodePorts: ptr.To(true),
			}},
		},
		{
			name: "Expected InternalTrafficPolicy/ExternalTrafficPolicy/AllocateLoadBalancerPorts are used if not empty",
			args: args{
				expected: corev1.Service{Spec: corev1.ServiceSpec{
					InternalTrafficPolicy:         pointer(corev1.ServiceInternalTrafficPolicyLocal),
					ExternalTrafficPolicy:         corev1.ServiceExternalTrafficPolicyLocal,
					AllocateLoadBalancerNodePorts: ptr.To(false),
				}},
				reconciled: corev1.Service{Spec: corev1.ServiceSpec{
					InternalTrafficPolicy:         pointer(corev1.ServiceInternalTrafficPolicyCluster),
					ExternalTrafficPolicy:         corev1.ServiceExternalTrafficPolicyCluster,
					AllocateLoadBalancerNodePorts: ptr.To(true),
				}},
			},
			want: corev1.Service{Spec: corev1.ServiceSpec{
				InternalTrafficPolicy:         pointer(corev1.ServiceInternalTrafficPolicyLocal),
				ExternalTrafficPolicy:         corev1.ServiceExternalTrafficPolicyLocal,
				AllocateLoadBalancerNodePorts: ptr.To(false),
			}},
		},
		{
			name: "Reconciled LoadBalancerClass is used if the expected one is empty",
			args: args{
				expected: corev1.Service{Spec: corev1.ServiceSpec{}},
				reconciled: corev1.Service{Spec: corev1.ServiceSpec{
					Type:              corev1.ServiceTypeLoadBalancer,
					LoadBalancerClass: ptr.To("service.k8s.aws/nlb"),
				}},
			},
			want: corev1.Service{Spec: corev1.ServiceSpec{
				Type:              corev1.ServiceTypeLoadBalancer,
				LoadBalancerClass: ptr.To("service.k8s.aws/nlb"),
			}},
		},
		{
			name: "Expected LoadBalancerClass is used if not empty",
			args: args{
				expected: corev1.Service{Spec: corev1.ServiceSpec{
					Type:              corev1.ServiceTypeLoadBalancer,
					LoadBalancerClass: ptr.To("explicit.lb/class"),
				}},
				reconciled: corev1.Service{Spec: corev1.ServiceSpec{}},
			},
			want: corev1.Service{Spec: corev1.ServiceSpec{
				Type:              corev1.ServiceTypeLoadBalancer,
				LoadBalancerClass: ptr.To("explicit.lb/class"),
			}},
		},
		{
			name: "Expected LoadBalancerClass can be reset if Type is no longer LoadBalancer",
			args: args{
				expected: corev1.Service{Spec: corev1.ServiceSpec{
					Type:              corev1.ServiceTypeClusterIP,
					LoadBalancerClass: nil,
				}},
				reconciled: corev1.Service{Spec: corev1.ServiceSpec{
					Type:              corev1.ServiceTypeLoadBalancer,
					LoadBalancerClass: ptr.To("service.k8s.aws/nlb"),
				}},
			},
			want: corev1.Service{Spec: corev1.ServiceSpec{
				Type: corev1.ServiceTypeClusterIP,
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyServerSideValues(&tt.args.expected, &tt.args.reconciled)
			compare.JSONEqual(t, tt.want, tt.args.expected)
		})
	}
}

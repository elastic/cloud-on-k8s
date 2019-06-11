// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestNeedsUpdate(t *testing.T) {
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = needsUpdate(&tt.args.expected, &tt.args.reconciled)
			if !reflect.DeepEqual(tt.args.expected, tt.want) {
				t.Errorf("needsUpdate(expected, reconcilied); expected is %v, want %v", tt.args.expected, tt.want)
			}
		})
	}
}

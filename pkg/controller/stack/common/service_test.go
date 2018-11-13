package common

import (
	"reflect"
	"testing"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

func TestGetElasticsearchServiceType(t *testing.T) {
	type args struct {
		s deploymentsv1alpha1.Stack
	}
	tests := []struct {
		name string
		args args
		want corev1.ServiceType
	}{
		{
			name: "Empty Expose means ClusterIP service type",
			args: args{s: deploymentsv1alpha1.Stack{}},
			want: corev1.ServiceTypeClusterIP,
		},
		{
			name: "Expose with a correct value returns it",
			args: args{s: deploymentsv1alpha1.Stack{
				Spec: deploymentsv1alpha1.StackSpec{
					Elasticsearch: deploymentsv1alpha1.ElasticsearchSpec{
						Expose: "NodePort",
					},
				},
			}},
			want: corev1.ServiceTypeNodePort,
		},
		{
			name: "Expose with a correct value returns it",
			args: args{s: deploymentsv1alpha1.Stack{
				Spec: deploymentsv1alpha1.StackSpec{
					Elasticsearch: deploymentsv1alpha1.ElasticsearchSpec{
						Expose: "LoadBalancer",
					},
				},
			}},
			want: corev1.ServiceTypeLoadBalancer,
		},
		{
			name: "Expose with a correct value returns it",
			args: args{s: deploymentsv1alpha1.Stack{
				Spec: deploymentsv1alpha1.StackSpec{
					Elasticsearch: deploymentsv1alpha1.ElasticsearchSpec{
						Expose: "ClusterIP",
					},
				},
			}},
			want: corev1.ServiceTypeClusterIP,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetElasticsearchServiceType(tt.args.s); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetElasticsearchServiceType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetKibanaServiceType(t *testing.T) {
	type args struct {
		s deploymentsv1alpha1.Stack
	}
	tests := []struct {
		name string
		args args
		want corev1.ServiceType
	}{
		{
			name: "Empty Expose means ClusterIP service type",
			args: args{s: deploymentsv1alpha1.Stack{}},
			want: corev1.ServiceTypeClusterIP,
		},
		{
			name: "Expose with a correct value returns it",
			args: args{s: deploymentsv1alpha1.Stack{
				Spec: deploymentsv1alpha1.StackSpec{
					Kibana: deploymentsv1alpha1.KibanaSpec{
						Expose: "NodePort",
					},
				},
			}},
			want: corev1.ServiceTypeNodePort,
		},
		{
			name: "Expose with a correct value returns it",
			args: args{s: deploymentsv1alpha1.Stack{
				Spec: deploymentsv1alpha1.StackSpec{
					Kibana: deploymentsv1alpha1.KibanaSpec{
						Expose: "LoadBalancer",
					},
				},
			}},
			want: corev1.ServiceTypeLoadBalancer,
		},
		{
			name: "Expose with a correct value returns it",
			args: args{s: deploymentsv1alpha1.Stack{
				Spec: deploymentsv1alpha1.StackSpec{
					Kibana: deploymentsv1alpha1.KibanaSpec{
						Expose: "ClusterIP",
					},
				},
			}},
			want: corev1.ServiceTypeClusterIP,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetKibanaServiceType(tt.args.s); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetKibanaServiceType() = %v, want %v", got, tt.want)
			}
		})
	}
}

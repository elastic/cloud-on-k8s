// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestGetServiceType(t *testing.T) {
	type args struct {
		s string
	}
	tests := []struct {
		name string
		args args
		want corev1.ServiceType
	}{
		{
			name: "Empty Type means ClusterIP service type",
			args: args{s: ""},
			want: corev1.ServiceTypeClusterIP,
		},
		{
			name: "Type with a correct value returns it",
			args: args{s: "NodePort"},
			want: corev1.ServiceTypeNodePort,
		},
		{
			name: "Type with a correct value returns it",
			args: args{s: "LoadBalancer"},
			want: corev1.ServiceTypeLoadBalancer,
		},
		{
			name: "Type with a correct value returns it",
			args: args{s: "ClusterIP"},
			want: corev1.ServiceTypeClusterIP,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetServiceType(tt.args.s); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetServiceType() = %v, want %v", got, tt.want)
			}
		})
	}
}

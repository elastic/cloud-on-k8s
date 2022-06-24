// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_esReachableConditionMessage(t *testing.T) {
	type args struct {
		internalService        *corev1.Service
		isServiceReady         bool
		isRespondingToRequests bool
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			args: args{
				internalService:        &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "name", Namespace: "namespace"}},
				isServiceReady:         false,
				isRespondingToRequests: false,
			},
			want: "Service namespace/name has no endpoint",
		},
		{
			args: args{
				internalService:        &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "name", Namespace: "namespace"}},
				isServiceReady:         true,
				isRespondingToRequests: false,
			},
			want: "Service namespace/name has endpoints but Elasticsearch is unavailable",
		},
		{
			args: args{
				internalService:        &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "name", Namespace: "namespace"}},
				isServiceReady:         true,
				isRespondingToRequests: true,
			},
			want: "Service namespace/name has endpoints",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := esReachableConditionMessage(tt.args.internalService, tt.args.isServiceReady, tt.args.isRespondingToRequests); got != tt.want {
				t.Errorf("esReachableConditionMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}

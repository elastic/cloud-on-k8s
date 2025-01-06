// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHTTPService(t *testing.T) {
	type args struct {
		kbName string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "sample",
			args: args{kbName: "sample"},
			want: "sample-kb-http",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HTTPService(tt.args.kbName); got != tt.want {
				t.Errorf("HTTPService() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigSecret(t *testing.T) {
	type args struct {
		kb Kibana
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "sample",
			args: args{kb: Kibana{ObjectMeta: metav1.ObjectMeta{Name: "sample"}}},
			want: "sample-kb-config",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ConfigSecret(tt.args.kb); got != tt.want {
				t.Errorf("ConfigSecret() = %v, want %v", got, tt.want)
			}
		})
	}
}

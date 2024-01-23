// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	"testing"
)

func TestAPIService(t *testing.T) {
	type args struct {
		logstashName string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "sample",
			args: args{logstashName: "sample"},
			want: "sample-ls-api",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := APIServiceName(tt.args.logstashName); got != tt.want {
				t.Errorf("DefaultService() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigSecretName(t *testing.T) {
	type args struct {
		logstashName string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "sample",
			args: args{logstashName: "sample"},
			want: "sample-ls-config",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ConfigSecretName(tt.args.logstashName); got != tt.want {
				t.Errorf("ConfigSecret() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLogstashName(t *testing.T) {
	type args struct {
		logstashName string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "sample",
			args: args{logstashName: "sample"},
			want: "sample-ls",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Name(tt.args.logstashName); got != tt.want {
				t.Errorf("Logstash Name() = %v, want %v", got, tt.want)
			}
		})
	}
}

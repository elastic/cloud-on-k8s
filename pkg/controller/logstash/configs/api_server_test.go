// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package configs

import "testing"

func TestAPIServer_UsesBasicAuth(t *testing.T) {
	tests := []struct {
		name     string
		authType string
		want     bool
	}{
		{name: "lowercase basic", authType: "basic", want: true},
		{name: "titlecase Basic", authType: "Basic", want: true},
		{name: "uppercase BASIC", authType: "BASIC", want: true},
		{name: "empty auth type", authType: "", want: false},
		{name: "other auth type", authType: "none", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := APIServer{AuthType: tt.authType}
			if got := s.UsesBasicAuth(); got != tt.want {
				t.Errorf("UsesBasicAuth() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAPIServer_UseTLS(t *testing.T) {
	tests := []struct {
		name       string
		sslEnabled string
		want       bool
	}{
		{name: "empty defaults to TLS on", sslEnabled: "", want: true},
		{name: "explicit true", sslEnabled: "true", want: true},
		{name: "explicit false", sslEnabled: "false", want: false},
		{name: "unrecognised value defaults to off", sslEnabled: "yes", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := APIServer{SSLEnabled: tt.sslEnabled}
			if got := s.UseTLS(); got != tt.want {
				t.Errorf("UseTLS() = %v, want %v", got, tt.want)
			}
		})
	}
}

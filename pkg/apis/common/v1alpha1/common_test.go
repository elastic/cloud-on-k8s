// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import "testing"

func TestTLSOptions_Enabled(t *testing.T) {
	type fields struct {
		SelfSignedCertificate *SelfSignedCertificate
		Certificate           SecretRef
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{
			name: "disabled: no custom cert and self-signed disabled",
			fields: fields{
				SelfSignedCertificate: &SelfSignedCertificate{
					Disabled: true,
				},
				Certificate: SecretRef{},
			},
			want: false,
		},
		{
			name: "enabled: custom certs and self-signed disabled",
			fields: fields{
				SelfSignedCertificate: &SelfSignedCertificate{
					Disabled: true,
				},
				Certificate: SecretRef{
					SecretName: "my-custom-certs",
				},
			},
			want: true,
		},
		{
			name:   "enabled: by default",
			fields: fields{},
			want:   true,
		},
		{
			name: "enabled: via self-signed certificates",
			fields: fields{
				SelfSignedCertificate: &SelfSignedCertificate{
					SubjectAlternativeNames: []SubjectAlternativeName{},
					Disabled:                false,
				},
				Certificate: SecretRef{},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tls := TLSOptions{
				SelfSignedCertificate: tt.fields.SelfSignedCertificate,
				Certificate:           tt.fields.Certificate,
			}
			if got := tls.Enabled(); got != tt.want {
				t.Errorf("TLSOptions.Enabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHTTPConfig_Scheme(t *testing.T) {
	type fields struct {
		TLS TLSOptions
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "enabled",
			fields: fields{
				TLS: TLSOptions{
					SelfSignedCertificate: &SelfSignedCertificate{
						Disabled: false,
					},
				},
			},
			want: "https",
		},
		{
			name: "disabled",
			fields: fields{
				TLS: TLSOptions{
					SelfSignedCertificate: &SelfSignedCertificate{
						Disabled: true,
					},
				},
			},
			want: "http",
		},
		{
			name: "default",
			fields: fields{
				TLS: TLSOptions{},
			},
			want: "https",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			http := HTTPConfig{
				TLS: tt.fields.TLS,
			}
			if got := http.Scheme(); got != tt.want {
				t.Errorf("Scheme() = %v, want %v", got, tt.want)
			}
		})
	}
}

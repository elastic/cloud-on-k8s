// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package config

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/apm/v1alpha1"
	alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
)

func TestNewConfigFromSpec(t *testing.T) {
	type args struct {
		as v1alpha1.ApmServer
		c  k8s.Client
	}
	tests := []struct {
		name    string
		args    args
		want    *settings.CanonicalConfig
		wantErr bool
	}{
		{
			name: "default config",
			want: settings.MustCanonicalConfig(map[string]interface{}{
				APMServerHost:           ":8200",
				APMServerSecretToken:    "${SECRET_TOKEN}",
				APMServerSSLEnabled:     true,
				APMServerSSLCertificate: "/mnt/elastic-internal/http-certs/tls.crt",
				APMServerSSLKey:         "/mnt/elastic-internal/http-certs/tls.key",
			}),
			wantErr: false,
		},
		{
			name: "TLS turned off",
			args: args{
				as: v1alpha1.ApmServer{
					Spec: v1alpha1.ApmServerSpec{
						HTTP: alpha1.HTTPConfig{
							TLS: alpha1.TLSOptions{
								SelfSignedCertificate: &alpha1.SelfSignedCertificate{
									Disabled: true,
								},
							},
						},
					},
				},
			},
			want: settings.MustCanonicalConfig(map[string]interface{}{
				APMServerHost:        ":8200",
				APMServerSecretToken: "${SECRET_TOKEN}",
			}),
			wantErr: false,
		},
		{
			name: "With User config (overriding ECK settings)",
			args: args{
				as: v1alpha1.ApmServer{
					Spec: v1alpha1.ApmServerSpec{
						Config: &alpha1.Config{
							Data: map[string]interface{}{
								APMServerSSLEnabled: false,
							},
						},
					},
				},
			},
			want: settings.MustCanonicalConfig(map[string]interface{}{
				APMServerHost:           ":8200",
				APMServerSecretToken:    "${SECRET_TOKEN}",
				APMServerSSLEnabled:     false,
				APMServerSSLCertificate: "/mnt/elastic-internal/http-certs/tls.crt",
				APMServerSSLKey:         "/mnt/elastic-internal/http-certs/tls.key",
			}),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewConfigFromSpec(tt.args.c, tt.args.as)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewConfigFromSpec() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := got.Diff(tt.want, nil); diff != nil {
				t.Error(diff)
			}
		})
	}
}

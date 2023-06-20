// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package apmserver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apmv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func TestNewConfigFromSpec(t *testing.T) {
	testCases := []struct {
		name            string
		configOverrides map[string]interface{}
		esAssocConf     *commonv1.AssociationConf
		kbAssocConf     *commonv1.AssociationConf
		version         version.Version
		wantConf        map[string]interface{}
		wantErr         bool
	}{
		{
			name:    "default config",
			version: version.MinFor(8, 0, 0),
			wantConf: map[string]interface{}{
				"apm-server.auth.secret_token": "${SECRET_TOKEN}",
			},
		},
		{
			name:    "default config pre 8.0",
			version: version.MinFor(7, 0, 0),
			wantConf: map[string]interface{}{
				"apm-server.secret_token": "${SECRET_TOKEN}",
			},
		},
		{
			name: "with overridden config",
			configOverrides: map[string]interface{}{
				"apm-server.secret_token": "MYSECRET",
			},
			wantConf: map[string]interface{}{
				"apm-server.secret_token": "MYSECRET",
			},
			version: version.MinFor(7, 0, 0),
		},
		{
			name: "without Elasticsearch CA cert",
			esAssocConf: &commonv1.AssociationConf{
				AuthSecretName: "test-es-elastic-user",
				AuthSecretKey:  "elastic",
				CASecretName:   "test-es-http-ca-public",
				CACertProvided: false,
				URL:            "https://test-es-http.default.svc:9200",
			},
			wantConf: map[string]interface{}{
				// version specific auth token
				"apm-server.auth.secret_token":  "${SECRET_TOKEN}",
				"output.elasticsearch.hosts":    []string{"https://test-es-http.default.svc:9200"},
				"output.elasticsearch.username": "elastic",
				"output.elasticsearch.password": "password",
			},
			version: version.MinFor(8, 0, 0),
		},
		{
			name: "with Elasticsearch CA cert",
			esAssocConf: &commonv1.AssociationConf{
				AuthSecretName: "test-es-elastic-user",
				AuthSecretKey:  "elastic",
				CASecretName:   "test-es-http-ca-public",
				CACertProvided: true,
				URL:            "https://test-es-http.default.svc:9200",
			},
			wantConf: map[string]interface{}{
				"apm-server.auth.secret_token":                     "${SECRET_TOKEN}",
				"output.elasticsearch.hosts":                       []string{"https://test-es-http.default.svc:9200"},
				"output.elasticsearch.username":                    "elastic",
				"output.elasticsearch.password":                    "password",
				"output.elasticsearch.ssl.certificate_authorities": []string{"config/elasticsearch-certs/ca.crt"},
			},
			version: version.MinFor(8, 0, 0),
		},
		{
			name: "missing auth secret",
			esAssocConf: &commonv1.AssociationConf{
				AuthSecretName: "wrong-secret",
				AuthSecretKey:  "elastic",
				CASecretName:   "test-es-http-ca-public",
				CACertProvided: true,
				URL:            "https://test-es-http.default.svc:9200",
			},
			wantErr: true,
			version: version.MinFor(8, 0, 0),
		},
		{
			name: "Kibana and Elasticsearch configuration",
			esAssocConf: &commonv1.AssociationConf{
				AuthSecretName: "test-es-elastic-user",
				AuthSecretKey:  "elastic",
				CASecretName:   "test-es-http-ca-public",
				CACertProvided: true,
				URL:            "https://test-es-http.default.svc:9200",
			},
			kbAssocConf: &commonv1.AssociationConf{
				AuthSecretName: "test-kb-elastic-user",
				AuthSecretKey:  "apm-kb-user",
				CASecretName:   "test-kb-http-ca-public",
				CACertProvided: true,
				URL:            "https://test-kb-http.default.svc:9200",
			},
			wantConf: map[string]interface{}{
				// Elasticsearch configuration
				"output.elasticsearch.hosts":                       []string{"https://test-es-http.default.svc:9200"},
				"output.elasticsearch.username":                    "elastic",
				"output.elasticsearch.password":                    "password",
				"output.elasticsearch.ssl.certificate_authorities": []string{"config/elasticsearch-certs/ca.crt"},
				// Kibana configuration
				"apm-server.kibana.enabled":                     true,
				"apm-server.kibana.host":                        "https://test-kb-http.default.svc:9200",
				"apm-server.kibana.username":                    "apm-kb-user",
				"apm-server.kibana.password":                    "password-kb-user",
				"apm-server.kibana.ssl.certificate_authorities": []string{"config/kibana-certs/ca.crt"},
				// version specific auth token
				"apm-server.auth.secret_token": "${SECRET_TOKEN}",
			},
			version: version.MinFor(8, 0, 0),
		},
		{
			name: "Elasticsearch fully configured and Kibana configuration without CA",
			esAssocConf: &commonv1.AssociationConf{
				AuthSecretName: "test-es-elastic-user",
				AuthSecretKey:  "elastic",
				CASecretName:   "test-es-http-ca-public",
				CACertProvided: true,
				URL:            "https://test-es-http.default.svc:9200",
			},
			kbAssocConf: &commonv1.AssociationConf{
				AuthSecretName: "test-kb-elastic-user",
				AuthSecretKey:  "apm-kb-user",
				CASecretName:   "test-kb-http-ca-public",
				CACertProvided: false,
				URL:            "https://test-kb-http.default.svc:9200",
			},
			wantConf: map[string]interface{}{
				// Elasticsearch configuration
				"output.elasticsearch.hosts":                       []string{"https://test-es-http.default.svc:9200"},
				"output.elasticsearch.username":                    "elastic",
				"output.elasticsearch.password":                    "password",
				"output.elasticsearch.ssl.certificate_authorities": []string{"config/elasticsearch-certs/ca.crt"},
				// Kibana configuration
				"apm-server.kibana.enabled":  true,
				"apm-server.kibana.host":     "https://test-kb-http.default.svc:9200",
				"apm-server.kibana.username": "apm-kb-user",
				"apm-server.kibana.password": "password-kb-user",
				// version specific auth token
				"apm-server.auth.secret_token": "${SECRET_TOKEN}",
			},
			version: version.MinFor(8, 0, 0),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := k8s.NewFakeClient(mkAuthSecrets()...)
			apmServer := &apmv1.ApmServer{
				ObjectMeta: metav1.ObjectMeta{
					Name: "apm-server",
				},
				Spec: apmv1.ApmServerSpec{
					Config: &commonv1.Config{Data: tc.configOverrides},
				},
			}

			apmv1.NewApmEsAssociation(apmServer).SetAssociationConf(tc.esAssocConf)
			apmv1.NewApmKibanaAssociation(apmServer).SetAssociationConf(tc.kbAssocConf)

			gotConf, err := newConfigFromSpec(context.Background(), client, apmServer, tc.version)
			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			wantConf := mkConf(t, tc.wantConf)
			diff := wantConf.Diff(gotConf, nil)
			require.Len(t, diff, 0)
		})
	}
}

func mkAuthSecrets() []client.Object {
	return []client.Object{
		&v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-es-elastic-user",
			},
			Data: map[string][]byte{
				"elastic": []byte("password"),
			},
		},
		&v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-kb-elastic-user",
			},
			Data: map[string][]byte{
				"apm-kb-user": []byte("password-kb-user"),
			},
		},
	}
}

func mkConf(t *testing.T, overrides map[string]interface{}) *settings.CanonicalConfig {
	t.Helper()
	cfg, err := settings.NewCanonicalConfigFrom(map[string]interface{}{
		"apm-server.host":            ":8200",
		"apm-server.ssl.certificate": "/mnt/elastic-internal/http-certs/tls.crt",
		"apm-server.ssl.enabled":     true,
		"apm-server.ssl.key":         "/mnt/elastic-internal/http-certs/tls.key",
	})
	require.NoError(t, err)

	overriddenCfg, err := settings.NewCanonicalConfigFrom(overrides)
	require.NoError(t, err)

	require.NoError(t, cfg.MergeWith(overriddenCfg))
	return cfg
}

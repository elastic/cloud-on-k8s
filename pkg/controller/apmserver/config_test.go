// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserver

import (
	"testing"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestNewConfigFromSpec(t *testing.T) {
	testCases := []struct {
		name            string
		configOverrides map[string]interface{}
		esAssocConf     *commonv1.AssociationConf
		kbAssocConf     *commonv1.AssociationConf
		wantConf        map[string]interface{}
		wantErr         bool
	}{
		{
			name: "default config",
		},
		{
			name: "with overridden config",
			configOverrides: map[string]interface{}{
				"apm-server.secret_token": "MYSECRET",
			},
			wantConf: map[string]interface{}{
				"apm-server.secret_token": "MYSECRET",
			},
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
				"output.elasticsearch.hosts":    []string{"https://test-es-http.default.svc:9200"},
				"output.elasticsearch.username": "elastic",
				"output.elasticsearch.password": "password",
			},
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
				"output.elasticsearch.hosts":                       []string{"https://test-es-http.default.svc:9200"},
				"output.elasticsearch.username":                    "elastic",
				"output.elasticsearch.password":                    "password",
				"output.elasticsearch.ssl.certificate_authorities": []string{"config/elasticsearch-certs/ca.crt"},
			},
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
			},
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
			},
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

			gotConf, err := newConfigFromSpec(client, apmServer)
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

func mkAuthSecrets() []runtime.Object {
	return []runtime.Object{
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
		"apm-server.secret_token":    "${SECRET_TOKEN}",
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

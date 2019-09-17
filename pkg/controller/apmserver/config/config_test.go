// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package config

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1alpha1"
	commonv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestNewConfigFromSpec(t *testing.T) {
	testCases := []struct {
		name            string
		configOverrides map[string]interface{}
		assocConf       *commonv1alpha1.AssociationConf
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
			assocConf: &commonv1alpha1.AssociationConf{
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
			assocConf: &commonv1alpha1.AssociationConf{
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
			assocConf: &commonv1alpha1.AssociationConf{
				AuthSecretName: "wrong-secret",
				AuthSecretKey:  "elastic",
				CASecretName:   "test-es-http-ca-public",
				CACertProvided: true,
				URL:            "https://test-es-http.default.svc:9200",
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := k8s.WrapClient(fake.NewFakeClient(mkAuthSecret()))
			apmServer := mkAPMServer(tc.configOverrides, tc.assocConf)
			gotConf, err := NewConfigFromSpec(client, apmServer)
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

func mkAPMServer(config map[string]interface{}, assocConf *commonv1alpha1.AssociationConf) *v1alpha1.ApmServer {
	apmServer := &v1alpha1.ApmServer{
		ObjectMeta: metav1.ObjectMeta{
			Name: "apm-server",
		},
		Spec: v1alpha1.ApmServerSpec{
			Config: &commonv1alpha1.Config{Data: config},
		},
	}

	apmServer.SetAssociationConf(assocConf)
	return apmServer
}

func mkAuthSecret() *v1.Secret {
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-es-elastic-user",
		},
		Data: map[string][]byte{
			"elastic": []byte("password"),
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

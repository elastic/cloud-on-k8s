// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	commonsettings "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/nodespec"
)

func Test_detectClientAuthenticationRequired(t *testing.T) {
	ver, err := version.Parse("8.17.0")
	require.NoError(t, err)

	newES := func(specAuthEnabled bool, userConfig *commonv1.Config) esv1.Elasticsearch {
		ns := esv1.NodeSet{Name: "default", Count: 1}
		if userConfig != nil {
			ns.Config = userConfig
		}
		return esv1.Elasticsearch{
			ObjectMeta: metav1.ObjectMeta{Name: "test-es", Namespace: "default"},
			Spec: esv1.ElasticsearchSpec{
				Version: "8.17.0",
				HTTP: commonv1.HTTPConfigWithClientOptions{
					TLS: commonv1.TLSWithClientOptions{
						Client: commonv1.ClientOptions{Authentication: specAuthEnabled},
					},
				},
				NodeSets: []esv1.NodeSet{ns},
			},
		}
	}

	policyWithOverride := func(val string) nodespec.PolicyConfig {
		return nodespec.PolicyConfig{
			ElasticsearchConfig: commonsettings.MustCanonicalConfig(map[string]any{
				esv1.XPackSecurityHttpSslClientAuthentication: val,
			}),
		}
	}

	tests := []struct {
		name                      string
		es                        esv1.Elasticsearch
		policyConfig              nodespec.PolicyConfig
		enterpriseFeaturesEnabled bool
		wantRequired              bool
		wantWarningContains       string
	}{
		{
			name:                      "enterprise disabled: client auth not detected even with spec field set",
			es:                        newES(true, nil),
			enterpriseFeaturesEnabled: false,
		},
		{
			name:                      "enterprise enabled: client auth detected when spec field is set",
			es:                        newES(true, nil),
			enterpriseFeaturesEnabled: true,
			wantRequired:              true,
		},
		{
			name:                      "enterprise enabled: warning when policy overrides to optional",
			es:                        newES(true, nil),
			policyConfig:              policyWithOverride("optional"),
			enterpriseFeaturesEnabled: true,
			wantWarningContains:       "ineffective due to StackConfigPolicy configuration",
		},
		{
			name:                      "enterprise enabled: no warning when policy keeps required",
			es:                        newES(true, nil),
			policyConfig:              policyWithOverride("required"),
			enterpriseFeaturesEnabled: true,
			wantRequired:              true,
		},
		{
			name: "enterprise enabled: warning when user manual config overrides to optional",
			es: newES(true, &commonv1.Config{Data: map[string]any{
				esv1.XPackSecurityHttpSslClientAuthentication: "optional",
			}}),
			enterpriseFeaturesEnabled: true,
			wantWarningContains:       "ineffective due to User manual configuration",
		},
		{
			name:                      "enterprise enabled: no warning when spec field is disabled",
			es:                        newES(false, nil),
			policyConfig:              policyWithOverride("optional"),
			enterpriseFeaturesEnabled: true,
		},
		{
			name:                      "enterprise enabled: no warning when override config is nil",
			es:                        newES(true, nil),
			enterpriseFeaturesEnabled: true,
			wantRequired:              true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			required, warning, err := detectClientAuthenticationRequired(tt.es, ver, corev1.IPv4Protocol, tt.policyConfig, tt.enterpriseFeaturesEnabled)
			require.NoError(t, err)
			assert.Equal(t, tt.wantRequired, required)
			if tt.wantWarningContains != "" {
				assert.Contains(t, warning, tt.wantWarningContains)
			} else {
				assert.Empty(t, warning)
			}
		})
	}
}

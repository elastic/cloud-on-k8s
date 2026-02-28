// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
	"testing"

	"github.com/stretchr/testify/require"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	common "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
)

func TestClientAuthenticationSpecIneffectiveWarning(t *testing.T) {
	es := esv1.Elasticsearch{
		Spec: esv1.ElasticsearchSpec{
			HTTP: commonv1.HTTPConfigWithClientOptions{
				TLS: commonv1.TLSWithClientOptions{
					Client: commonv1.ClientOptions{Authentication: true},
				},
			},
		},
	}

	t.Run("returns warning when policy overrides to optional", func(t *testing.T) {
		policyCfg := common.MustCanonicalConfig(map[string]any{
			esv1.XPackSecurityHttpSslClientAuthentication: "optional",
		})

		warning := clientAuthenticationSpecIneffectiveWarning(es.Spec.HTTP.TLS.Client.Authentication, policyCfg, "StackConfigPolicy")
		require.Contains(t, warning, "spec.http.tls.client.authentication is ineffective due to StackConfigPolicy configuration")
		require.Contains(t, warning, esv1.XPackSecurityHttpSslClientAuthentication)
		require.Contains(t, warning, "optional")
	})

	t.Run("returns empty when policy keeps required", func(t *testing.T) {
		policyCfg := common.MustCanonicalConfig(map[string]any{
			esv1.XPackSecurityHttpSslClientAuthentication: "required",
		})

		warning := clientAuthenticationSpecIneffectiveWarning(es.Spec.HTTP.TLS.Client.Authentication, policyCfg, "StackConfigPolicy")
		require.Empty(t, warning)
	})

	t.Run("returns warning when manual config overrides to optional", func(t *testing.T) {
		userCfg := common.MustCanonicalConfig(map[string]any{
			esv1.XPackSecurityHttpSslClientAuthentication: "optional",
		})

		warning := clientAuthenticationSpecIneffectiveWarning(es.Spec.HTTP.TLS.Client.Authentication, userCfg, "User manual")
		require.Contains(t, warning, "ineffective due to User manual configuration")
		require.Contains(t, warning, "optional")
	})

	t.Run("returns empty when spec field is disabled", func(t *testing.T) {
		es.Spec.HTTP.TLS.Client.Authentication = false
		policyCfg := common.MustCanonicalConfig(map[string]any{
			esv1.XPackSecurityHttpSslClientAuthentication: "optional",
		})

		warning := clientAuthenticationSpecIneffectiveWarning(es.Spec.HTTP.TLS.Client.Authentication, policyCfg, "StackConfigPolicy")
		require.Empty(t, warning)
	})

	t.Run("returns empty when override config is nil", func(t *testing.T) {
		warning := clientAuthenticationSpecIneffectiveWarning(true, nil, "manual")
		require.Empty(t, warning)
	})
}

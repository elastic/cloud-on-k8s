// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
)

func TestCheckReservedPKIRealmConfig(t *testing.T) {
	path := field.NewPath("spec").Child("config")
	tests := []struct {
		name       string
		cfg        map[string]any
		wantFields []string
	}{
		{
			name: "no realms",
			cfg:  map[string]any{"node.store.allow_mmap": false},
		},
		{
			name: "realms with other orders",
			cfg: map[string]any{
				"xpack.security.authc.realms.jwt.jwt1.order":        10,
				"xpack.security.authc.realms.kerberos.krb1.order":   ECKPKIRealmOrder - 1,
				"xpack.security.authc.realms.saml.saml1.idp.entity": "https://idp",
			},
		},
		{
			name: "int order equal to the reserved order",
			cfg:  map[string]any{"xpack.security.authc.realms.jwt.jwt1.order": ECKPKIRealmOrder},
			wantFields: []string{
				"spec.config.xpack.security.authc.realms.jwt.jwt1.order",
			},
		},
		{
			name: "float64 order, as decoded from JSON, equal to the reserved order",
			cfg:  map[string]any{"xpack.security.authc.realms.jwt.jwt1.order": float64(ECKPKIRealmOrder)},
			wantFields: []string{
				"spec.config.xpack.security.authc.realms.jwt.jwt1.order",
			},
		},
		{
			name: "string order equal to the reserved order",
			cfg:  map[string]any{"xpack.security.authc.realms.jwt.jwt1.order": "2147483647"},
			wantFields: []string{
				"spec.config.xpack.security.authc.realms.jwt.jwt1.order",
			},
		},
		{
			name: "nested keys, several realms sharing the reserved order",
			cfg: map[string]any{
				"xpack": map[string]any{
					"security": map[string]any{
						"authc": map[string]any{
							"realms": map[string]any{
								"kerberos": map[string]any{"krb1": map[string]any{"order": ECKPKIRealmOrder}},
								"jwt":      map[string]any{"jwt1": map[string]any{"order": ECKPKIRealmOrder}},
								"ldap":     map[string]any{"ldap1": map[string]any{"order": 1}},
							},
						},
					},
				},
			},
			wantFields: []string{
				"spec.config.xpack.security.authc.realms.jwt.jwt1.order",
				"spec.config.xpack.security.authc.realms.kerberos.krb1.order",
			},
		},
		{
			name:       "reserved realm name (flat key)",
			cfg:        map[string]any{ECKPKIRealmConfigKey + ".enabled": false},
			wantFields: []string{"spec.config." + ECKPKIRealmConfigKey},
		},
		{
			name: "reserved realm name (nested key) with the reserved order",
			cfg: map[string]any{
				"xpack": map[string]any{
					"security": map[string]any{
						"authc": map[string]any{
							"realms": map[string]any{
								"pki": map[string]any{ECKPKIRealmName: map[string]any{"order": ECKPKIRealmOrder}},
							},
						},
					},
				},
			},
			wantFields: []string{
				"spec.config." + ECKPKIRealmConfigKey,
				"spec.config." + ECKPKIRealmConfigKey + ".order",
			},
		},
		{
			name:       "reserved realm name in another realm type (flat key)",
			cfg:        map[string]any{"xpack.security.authc.realms.ldap." + ECKPKIRealmName + ".order": 1},
			wantFields: []string{"spec.config.xpack.security.authc.realms.ldap." + ECKPKIRealmName},
		},
		{
			name: "reserved realm name in another realm type (nested key) with the reserved order",
			cfg: map[string]any{
				"xpack": map[string]any{
					"security": map[string]any{
						"authc": map[string]any{
							"realms": map[string]any{
								"file": map[string]any{ECKPKIRealmName: map[string]any{"order": ECKPKIRealmOrder}},
							},
						},
					},
				},
			},
			wantFields: []string{
				"spec.config.xpack.security.authc.realms.file." + ECKPKIRealmName,
				"spec.config.xpack.security.authc.realms.file." + ECKPKIRealmName + ".order",
			},
		},
		{
			name: "other pki realm name",
			cfg:  map[string]any{"xpack.security.authc.realms.pki.my_pki.order": 2},
		},
		{
			name:       "reserved node attribute (flat key)",
			cfg:        map[string]any{ECKPKIRealmNodeAttributeConfigKey: "true"},
			wantFields: []string{"spec.config." + ECKPKIRealmNodeAttributeConfigKey},
		},
		{
			name: "reserved node attribute (nested key) with the reserved realm name",
			cfg: map[string]any{
				"node":                            map[string]any{"attr": map[string]any{ECKPKIRealmNodeAttribute: "false"}},
				ECKPKIRealmConfigKey + ".enabled": false,
			},
			wantFields: []string{
				"spec.config." + ECKPKIRealmNodeAttributeConfigKey,
				"spec.config." + ECKPKIRealmConfigKey,
			},
		},
		{
			name: "other node attributes",
			cfg:  map[string]any{"node.attr.zone": "a", "node.attr." + ECKPKIRealmName: "true"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := settings.NewCanonicalConfigFrom(tt.cfg)
			require.NoError(t, err)
			errs := CheckReservedPKIRealmConfig(path, cfg)
			gotFields := make([]string, 0, len(errs))
			for _, e := range errs {
				require.Equal(t, field.ErrorTypeInvalid, e.Type)
				require.Contains(t, e.Detail, "reserved for")
				gotFields = append(gotFields, e.Field)
			}
			require.ElementsMatch(t, tt.wantFields, gotFields)
		})
	}
}

func TestCheckConsistentESRefAuthentication(t *testing.T) {
	ref := func(namespace, name string, mode AuthenticationMode) ElasticsearchSelector {
		return ElasticsearchSelector{Namespace: namespace, Name: name, AuthenticationMode: mode}
	}
	withCert := func(ref ElasticsearchSelector, secretName string) ElasticsearchSelector {
		ref.ClientCertificateSecretName = secretName
		return ref
	}
	tests := []struct {
		name       string
		refs       []ElasticsearchSelector
		wantFields []string
	}{
		{
			name: "no refs",
		},
		{
			name: "single PKI ref",
			refs: []ElasticsearchSelector{ref("", "es", AuthenticationModePKI)},
		},
		{
			name: "same ES with the same mode",
			refs: []ElasticsearchSelector{ref("", "es", AuthenticationModePKI), ref("ns", "es", AuthenticationModePKI)},
		},
		{
			name: "same ES with unset and Credentials modes",
			refs: []ElasticsearchSelector{ref("", "es", ""), ref("", "es", AuthenticationModeCredentials)},
		},
		{
			name: "different ES with different modes",
			refs: []ElasticsearchSelector{ref("", "es1", AuthenticationModePKI), ref("", "es2", AuthenticationModeCredentials)},
		},
		{
			name: "same name in different namespaces with different modes",
			refs: []ElasticsearchSelector{ref("", "es", AuthenticationModePKI), ref("other", "es", AuthenticationModeCredentials)},
		},
		{
			name:       "same ES with different modes",
			refs:       []ElasticsearchSelector{ref("", "es", AuthenticationModePKI), ref("", "es", "")},
			wantFields: []string{"spec.elasticsearchRefs[1].authenticationMode"},
		},
		{
			name: "same ES with different modes, with and without the default namespace",
			refs: []ElasticsearchSelector{
				ref("ns", "es", AuthenticationModeCredentials),
				ref("", "es1", AuthenticationModeCredentials),
				ref("", "es", AuthenticationModePKI),
			},
			wantFields: []string{"spec.elasticsearchRefs[2].authenticationMode"},
		},
		{
			name: "same ES in PKI mode with the same client certificate",
			refs: []ElasticsearchSelector{
				withCert(ref("", "es", AuthenticationModePKI), "cert"),
				withCert(ref("ns", "es", AuthenticationModePKI), "cert"),
			},
		},
		{
			name: "same ES in PKI mode with different client certificates",
			refs: []ElasticsearchSelector{
				withCert(ref("", "es", AuthenticationModePKI), "cert1"),
				withCert(ref("", "es", AuthenticationModePKI), "cert2"),
			},
			wantFields: []string{"spec.elasticsearchRefs[1].clientCertificateSecretName"},
		},
		{
			name: "same ES in PKI mode with a user-provided and an ECK-generated client certificate",
			refs: []ElasticsearchSelector{
				ref("", "es", AuthenticationModePKI),
				withCert(ref("", "es", AuthenticationModePKI), "cert"),
			},
			wantFields: []string{"spec.elasticsearchRefs[1].clientCertificateSecretName"},
		},
		{
			name: "same ES in Credentials mode with different client certificates",
			refs: []ElasticsearchSelector{
				withCert(ref("", "es", ""), "cert1"),
				withCert(ref("", "es", AuthenticationModeCredentials), "cert2"),
			},
		},
		{
			name: "different ES in PKI mode with different client certificates",
			refs: []ElasticsearchSelector{
				withCert(ref("", "es1", AuthenticationModePKI), "cert1"),
				withCert(ref("", "es2", AuthenticationModePKI), "cert2"),
			},
		},
		{
			name: "external refs are ignored",
			refs: []ElasticsearchSelector{
				{SecretName: "es"},
				ref("", "es", AuthenticationModePKI),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := CheckConsistentESRefAuthentication(field.NewPath("spec").Child("elasticsearchRefs"), "ns", tt.refs...)
			var gotFields []string
			for _, err := range errs {
				gotFields = append(gotFields, err.Field)
			}
			require.Equal(t, tt.wantFields, gotFields)
		})
	}
}

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
			name: "other pki realm name",
			cfg:  map[string]any{"xpack.security.authc.realms.pki.my_pki.order": 2},
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

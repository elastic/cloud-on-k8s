// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/test"
)

func TestWebhook(t *testing.T) {
	testCases := []test.ValidationWebhookTestCase{
		{
			Name:      "create-valid",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				m := mkStackConfigPolicy(uid)
				m.Spec.Elasticsearch.SecretMounts = []policyv1alpha1.SecretMount{
					{
						SecretName: "test1",
						MountPath:  "/usr/test1",
					},
					{
						SecretName: "test2",
						MountPath:  "/usr/test2",
					},
				}
				return test.MustMarshalJSON(t, m)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "create-valid-kibana",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				m := mkStackConfigPolicy(uid)
				m.Spec.Elasticsearch = policyv1alpha1.ElasticsearchConfigPolicySpec{}
				m.Spec.Kibana = policyv1alpha1.KibanaConfigPolicySpec{
					Config: &commonv1.Config{Data: map[string]any{"a": "b"}},
				}
				return test.MustMarshalJSON(t, m)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "unknown-field",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				m := mkStackConfigPolicy(uid)
				m.SetAnnotations(map[string]string{
					corev1.LastAppliedConfigAnnotation: `{
						"metadata":{"name": "scp", "namespace": "default", "uid": "e7a18cfb-b017-475c-8da2-1ec941b1f285", "creationTimestamp":"2020-03-24T13:43:20Z" },
						"spec":{"unknown": "blurb"}
					}`,
				})
				return test.MustMarshalJSON(t, m)
			},
			Check: test.ValidationWebhookFailed(
				`"unknown": unknown field found in the kubectl.kubernetes.io/last-applied-configuration annotation is unknown`,
			),
		},
		{
			Name:      "long-name",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				m := mkStackConfigPolicy(uid)
				m.SetName(strings.Repeat("x", 100))
				return test.MustMarshalJSON(t, m)
			},
			Check: test.ValidationWebhookFailed(
				`metadata.name: Too long: may not be more than 36 bytes`,
			),
		},
		{
			Name:      "no-settings",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				m := mkStackConfigPolicy(uid)
				m.Spec.Elasticsearch = policyv1alpha1.ElasticsearchConfigPolicySpec{
					SnapshotRepositories:      nil,
					SnapshotLifecyclePolicies: &commonv1.Config{Data: nil},
					ClusterSettings:           &commonv1.Config{Data: map[string]any{}},
				}
				return test.MustMarshalJSON(t, m)
			},
			Check: test.ValidationWebhookFailed(
				"One out of Elasticsearch or Kibana settings is mandatory, both must not be empty",
			),
		},
		{
			Name:      "create-with-deprecated-secure-settings",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				m := mkStackConfigPolicy(uid)
				m.Spec.SecureSettings = []commonv1.SecretSource{
					{SecretName: "my-secret"},
				}
				return test.MustMarshalJSON(t, m)
			},
			Check: test.ValidationWebhookSucceededWithWarnings(
				policyv1alpha1.SpecSecureSettingsDeprecated,
			),
		},
		{
			Name:      "create-duplicate-mountpaths",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				m := mkStackConfigPolicy(uid)
				m.Spec.Elasticsearch.SecretMounts = []policyv1alpha1.SecretMount{
					{
						SecretName: "test1",
						MountPath:  "/usr/test",
					},
					{
						SecretName: "test2",
						MountPath:  "/usr/test",
					},
				}
				return test.MustMarshalJSON(t, m)
			},
			Check: test.ValidationWebhookFailed(
				"SecretMounts cannot have duplicate mount paths",
			),
		},
		{
			// Validate also runs at reconcile time, where existing policies using the prefix must keep being applied.
			// Reserved names are rejected by the admission-only ValidateReservedRoleMappingNames instead.
			Name:      "reserved-eck-managed-pki-role-mapping-name-not-checked-by-validate",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				m := mkStackConfigPolicy(uid)
				m.Spec.Elasticsearch.SecurityRoleMappings = &commonv1.Config{Data: map[string]any{
					commonv1.ECKPKIRoleMappingPrefix + "custom": map[string]any{"enabled": true},
				}}
				return test.MustMarshalJSON(t, m)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "non-reserved-role-mapping-name",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				m := mkStackConfigPolicy(uid)
				m.Spec.Elasticsearch.SecurityRoleMappings = &commonv1.Config{Data: map[string]any{
					"custom_mapping": map[string]any{"enabled": true},
				}}
				return test.MustMarshalJSON(t, m)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			// The policy does not know if its target clusters enable PKI. Reserved realm settings are rejected by the
			// admission-only ValidateReservedPKIRealm, and at reconcile time only for clusters with PKI enabled.
			Name:      "reserved-eck-managed-pki-realm-not-checked-by-validate",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				m := mkStackConfigPolicy(uid)
				m.Spec.Elasticsearch.Config = &commonv1.Config{Data: map[string]any{
					commonv1.ECKPKIRealmConfigKey + ".enabled":     "false",
					"xpack.security.authc.realms.jwt.my_jwt.order": commonv1.ECKPKIRealmOrder,
				}}
				return test.MustMarshalJSON(t, m)
			},
			Check: test.ValidationWebhookSucceeded,
		},
	}

	handler := test.NewValidationWebhookHandler(policyv1alpha1.Validate)
	gvk := metav1.GroupVersionKind{Group: policyv1alpha1.GroupVersion.Group, Version: policyv1alpha1.GroupVersion.Version, Kind: policyv1alpha1.Kind}
	test.RunValidationWebhookTests(t, gvk, "stackconfigpolicies", handler, testCases...)
}

func TestValidateReservedRoleMappingNames(t *testing.T) {
	reserved := commonv1.ECKPKIRoleMappingPrefix + "corporate"
	otherReserved := commonv1.ECKPKIRoleMappingPrefix + "other"
	withRoleMappings := func(names ...string) *policyv1alpha1.StackConfigPolicy {
		p := mkStackConfigPolicy("uid")
		if names == nil {
			return p
		}
		data := map[string]any{}
		for _, name := range names {
			data[name] = map[string]any{"enabled": true}
		}
		p.Spec.Elasticsearch.SecurityRoleMappings = &commonv1.Config{Data: data}
		return p
	}

	tests := []struct {
		name         string
		policy       *policyv1alpha1.StackConfigPolicy
		old          *policyv1alpha1.StackConfigPolicy
		wantErrNames []string
	}{
		{
			name:   "create without role mappings",
			policy: withRoleMappings(),
		},
		{
			name:   "create with non-reserved names",
			policy: withRoleMappings("custom_mapping"),
		},
		{
			name:         "create with a reserved name",
			policy:       withRoleMappings("custom_mapping", reserved),
			wantErrNames: []string{reserved},
		},
		{
			name:   "update keeping a reserved name that predates the reservation",
			policy: withRoleMappings("custom_mapping", reserved),
			old:    withRoleMappings(reserved),
		},
		{
			name:         "update adding a reserved name next to an existing one",
			policy:       withRoleMappings(reserved, otherReserved),
			old:          withRoleMappings(reserved),
			wantErrNames: []string{otherReserved},
		},
		{
			name:         "update adding a reserved name to a policy without role mappings",
			policy:       withRoleMappings(reserved),
			old:          withRoleMappings(),
			wantErrNames: []string{reserved},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := policyv1alpha1.ValidateReservedRoleMappingNames(tt.policy, tt.old)
			var errMsg string
			if err != nil {
				errMsg = err.Error()
			}
			// Field errors are reported under spec.elasticsearch.securityRoleMappings[<name>].
			var gotErrNames []string
			for _, name := range []string{"custom_mapping", reserved, otherReserved} {
				if strings.Contains(errMsg, "["+name+"]") {
					gotErrNames = append(gotErrNames, name)
				}
			}
			require.ElementsMatch(t, tt.wantErrNames, gotErrNames)
			require.Equal(t, len(tt.wantErrNames) > 0, err != nil, "unexpected error state: %v", err)
		})
	}
}

func TestValidateReservedPKIRealm(t *testing.T) {
	reservedOrder := map[string]any{"xpack.security.authc.realms.jwt.my_jwt.order": commonv1.ECKPKIRealmOrder}
	tests := []struct {
		name    string
		config  map[string]any
		old     map[string]any // config of the old policy on update, nil on create
		wantErr string
	}{
		{
			name: "no config",
		},
		{
			name: "reserved eck_managed_pki realm name (flat key)",
			config: map[string]any{
				commonv1.ECKPKIRealmConfigKey + ".enabled": "false",
			},
			wantErr: "realm name eck_managed_pki is reserved for ECK-managed PKI authentication",
		},
		{
			name: "reserved eck_managed_pki realm name (nested key)",
			config: map[string]any{
				"xpack": map[string]any{
					"security": map[string]any{
						"authc": map[string]any{
							"realms": map[string]any{
								"pki": map[string]any{
									"eck_managed_pki": map[string]any{"enabled": false},
								},
							},
						},
					},
				},
			},
			wantErr: "realm name eck_managed_pki is reserved for ECK-managed PKI authentication",
		},
		{
			name: "user-defined pki realm with a different name",
			config: map[string]any{
				"xpack.security.authc.realms.pki.my_custom_pki.order": 5,
			},
		},
		{
			name: "reserved eck_managed_pki realm order",
			config: map[string]any{
				"xpack.security.authc.realms.jwt.my_jwt.order": commonv1.ECKPKIRealmOrder,
			},
			wantErr: "realm order 2147483647 is reserved for the ECK-managed PKI realm eck_managed_pki",
		},
		{
			name: "realm order below the reserved order",
			config: map[string]any{
				"xpack.security.authc.realms.jwt.my_jwt.order": commonv1.ECKPKIRealmOrder - 1,
			},
		},
		{
			name:   "update keeping a reserved realm order that predates the reservation",
			config: reservedOrder,
			old:    reservedOrder,
		},
		{
			name: "update keeping a reserved realm name that predates the reservation",
			config: map[string]any{
				commonv1.ECKPKIRealmConfigKey + ".enabled":     "false",
				"xpack.security.authc.realms.jwt.my_jwt.order": 1,
			},
			old: map[string]any{
				commonv1.ECKPKIRealmConfigKey + ".enabled": "false",
			},
		},
		{
			name:    "update introducing a reserved realm order",
			config:  reservedOrder,
			old:     map[string]any{"xpack.security.authc.realms.jwt.my_jwt.order": 1},
			wantErr: "realm order 2147483647 is reserved for the ECK-managed PKI realm eck_managed_pki",
		},
		{
			name: "update moving the reserved realm order to another realm",
			config: map[string]any{
				"xpack.security.authc.realms.jwt.other_jwt.order": commonv1.ECKPKIRealmOrder,
			},
			old:     reservedOrder,
			wantErr: "realm order 2147483647 is reserved for the ECK-managed PKI realm eck_managed_pki",
		},
		{
			name: "update keeping a reserved realm order and introducing a reserved realm name",
			config: map[string]any{
				commonv1.ECKPKIRealmConfigKey + ".enabled":     "false",
				"xpack.security.authc.realms.jwt.my_jwt.order": commonv1.ECKPKIRealmOrder,
			},
			old:     reservedOrder,
			wantErr: "realm name eck_managed_pki is reserved for ECK-managed PKI authentication",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := mkStackConfigPolicy("uid")
			if tt.config != nil {
				p.Spec.Elasticsearch.Config = &commonv1.Config{Data: tt.config}
			}
			var old *policyv1alpha1.StackConfigPolicy
			if tt.old != nil {
				old = mkStackConfigPolicy("uid")
				old.Spec.Elasticsearch.Config = &commonv1.Config{Data: tt.old}
			}
			err := policyv1alpha1.ValidateReservedPKIRealm(p, old)
			var errMsg string
			if err != nil {
				errMsg = err.Error()
			}
			require.Equal(t, tt.wantErr != "", err != nil, "unexpected error state: %v", err)
			require.Contains(t, errMsg, tt.wantErr)
		})
	}
}

func mkStackConfigPolicy(uid string) *policyv1alpha1.StackConfigPolicy {
	return &policyv1alpha1.StackConfigPolicy{
		Name: "config-policy-test",
		UID:  types.UID(uid),
		Spec: policyv1alpha1.StackConfigPolicySpec{
			Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
				ClusterSettings: &commonv1.Config{Data: map[string]any{"a": "b"}},
			},
		},
	}
}

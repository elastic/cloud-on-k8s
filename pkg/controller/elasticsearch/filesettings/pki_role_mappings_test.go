// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package filesettings

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/rolemapping"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func pkiTestCertificate(t *testing.T, name string, key *ecdsa.PrivateKey) ([]byte, *x509.Certificate) {
	t.Helper()
	if key == nil {
		var err error
		key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: name}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), cert
}

// pkiMappingKey returns the file-settings key for a PKI role mapping entry.
func pkiMappingKey(namespace, name string) string {
	return commonv1.ECKPKIRoleMappingPrefix + namespace + "." + name
}

// wantPKIRules returns the rules expected on the role mapping generated for fingerprint. It is written out literally,
// not built with pkiRoleMappingRules, so that a change to the generated rules fails the tests.
func wantPKIRules(fingerprint string) map[string]any {
	return map[string]any{
		"all": []any{
			map[string]any{"field": map[string]any{"metadata.pki_public_key_fingerprint": fingerprint}},
			map[string]any{"except": map[string]any{"field": map[string]any{"metadata.pki_delegated_by_realm": "*"}}},
		},
	}
}

// managedPKIEntry returns a role mapping entry carrying the metadata ECK sets on the PKI role mappings it generates.
func managedPKIEntry(uid string) map[string]any {
	return map[string]any{
		"enabled":  true,
		"roles":    []any{"kibana_system"},
		"metadata": map[string]any{"managed_by": "eck", "eck_resource_uid": uid},
	}
}

// userEntry returns a user-defined role mapping entry, as written by a StackConfigPolicy.
func userEntry(role string) map[string]any {
	return map[string]any{"enabled": true, "roles": []any{role}}
}

// fileSettingsSecretWithRoleMappings returns a file-settings Secret whose role_mappings are set to data.
func fileSettingsSecretWithRoleMappings(t *testing.T, es esv1.Elasticsearch, data map[string]any) *corev1.Secret {
	t.Helper()
	setupClient := k8s.NewFakeClient(&es)
	fs, err := Load(t.Context(), setupClient, k8s.ExtractNamespacedName(&es), metadata.Metadata{})
	require.NoError(t, err)
	fs.settings.State.RoleMappings = &commonv1.Config{Data: data}
	require.NoError(t, fs.Save(t.Context(), setupClient, &es))
	var secret corev1.Secret
	require.NoError(t, setupClient.Get(t.Context(), types.NamespacedName{Namespace: es.Namespace, Name: esv1.FileSettingsSecretName(es.Name)}, &secret))
	return &secret
}

func binding(ns, name, fingerprint, roles string) rolemapping.Request {
	return rolemapping.Request{
		SecretNamespace:      ns,
		SecretName:           name,
		PublicKeyFingerprint: fingerprint,
		Roles:                roles,
	}
}

func TestBuildPKIRoleMappings(t *testing.T) {
	_, certA := pkiTestCertificate(t, "my-kibana", nil)
	_, certB := pkiTestCertificate(t, "my-fleet-server", nil)
	fpA := certificates.PublicKeyFingerprint(certA)
	fpB := certificates.PublicKeyFingerprint(certB)

	tests := []struct {
		name string
		// runs holds one or more input orderings. All orderings are tested with the same
		// expected output — use multiple orderings to verify order-independence.
		runs          [][]rolemapping.Request
		wantNil       bool
		wantKeys      []string
		wantConflicts bool
		assertFn      func(t *testing.T, cfg *commonv1.Config)
	}{
		{
			name:    "empty bindings returns nil config",
			runs:    [][]rolemapping.Request{nil},
			wantNil: true,
		},
		{
			name:    "binding with empty roles is skipped",
			runs:    [][]rolemapping.Request{{binding("ns", "kb-client-cert", fpA, "")}},
			wantNil: true,
		},
		{
			name:    "binding with whitespace-only roles is skipped",
			runs:    [][]rolemapping.Request{{binding("ns", "kb-client-cert", fpA, "  ,  ")}},
			wantNil: true,
		},
		{
			name:     "single binding produces one mapping entry",
			runs:     [][]rolemapping.Request{{binding("ns", "kb-client-cert", fpA, "kibana_system")}},
			wantKeys: []string{pkiMappingKey("ns", "kb-client-cert")},
		},
		{
			name: "multiple bindings produce multiple entries",
			runs: [][]rolemapping.Request{{
				binding("ns", "kb-client-cert", fpA, "kibana_system"),
				binding("ns", "fleet-client-cert", fpB, "fleet_server"),
			}},
			wantKeys: []string{
				pkiMappingKey("ns", "kb-client-cert"),
				pkiMappingKey("ns", "fleet-client-cert"),
			},
		},
		{
			name:     "multi-role binding produces all roles in single entry",
			runs:     [][]rolemapping.Request{{binding("ns", "apm-client-cert", fpA, "apm_user_role_v87,apm_system")}},
			wantKeys: []string{pkiMappingKey("ns", "apm-client-cert")},
			assertFn: func(t *testing.T, cfg *commonv1.Config) {
				t.Helper()
				entry, ok := cfg.Data[pkiMappingKey("ns", "apm-client-cert")].(map[string]any)
				require.True(t, ok)
				require.ElementsMatch(t, []any{"apm_user_role_v87", "apm_system"}, entry["roles"])
			},
		},
		{
			name: "same secret name in two namespaces produces distinct mapping keys",
			runs: [][]rolemapping.Request{{
				binding("ns-a", "kb-client-cert", fpA, "kibana_system"),
				binding("ns-b", "kb-client-cert", fpB, "kibana_system"),
			}},
			wantKeys: []string{
				pkiMappingKey("ns-a", "kb-client-cert"),
				pkiMappingKey("ns-b", "kb-client-cert"),
			},
		},
		{
			name: "mapping entry has correct enabled/roles/fingerprint/metadata structure",
			runs: [][]rolemapping.Request{{
				{SecretNamespace: "ns", SecretName: "kb-client-cert", PublicKeyFingerprint: fpA, Roles: "kibana_system", ResourceUID: "uid-kb", AssociationName: "kb-es"},
			}},
			wantKeys: []string{pkiMappingKey("ns", "kb-client-cert")},
			assertFn: func(t *testing.T, cfg *commonv1.Config) {
				t.Helper()
				entry, ok := cfg.Data[pkiMappingKey("ns", "kb-client-cert")].(map[string]any)
				require.True(t, ok)
				require.Equal(t, true, entry["enabled"])
				require.Equal(t, []any{"kibana_system"}, entry["roles"])
				require.Equal(t, wantPKIRules(fpA), entry["rules"])
				meta, ok := entry["metadata"].(map[string]any)
				require.True(t, ok)
				require.Equal(t, "eck", meta["managed_by"])
				require.Equal(t, "uid-kb", meta["eck_resource_uid"])
				require.Equal(t, "kb-es", meta["eck_association"])
			},
		},
		{
			name: "two bindings produce independent entries with correct fingerprints",
			runs: [][]rolemapping.Request{{
				binding("ns", "kb-client-cert", fpA, "kibana_system"),
				binding("ns", "fleet-client-cert", fpB, "fleet_server"),
			}},
			wantKeys: []string{
				pkiMappingKey("ns", "kb-client-cert"),
				pkiMappingKey("ns", "fleet-client-cert"),
			},
			assertFn: func(t *testing.T, cfg *commonv1.Config) {
				t.Helper()
				entryA, ok := cfg.Data[pkiMappingKey("ns", "kb-client-cert")].(map[string]any)
				require.True(t, ok)
				require.Equal(t, wantPKIRules(fpA), entryA["rules"])

				entryB, ok := cfg.Data[pkiMappingKey("ns", "fleet-client-cert")].(map[string]any)
				require.True(t, ok)
				require.Equal(t, wantPKIRules(fpB), entryB["rules"])
			},
		},
		// Conflict cases: tested in both orderings to verify the detection is order-independent.
		{
			name: "conflict: same fingerprint with equivalent roles — both entries written",
			runs: [][]rolemapping.Request{
				{binding("ns", "first", fpA, "apm_system,apm_user_role_v87,apm_system"), binding("ns", "unrelated", fpB, "kibana_system"), binding("ns", "second", fpA, " apm_user_role_v87, apm_system, ")},
				{binding("ns", "second", fpA, " apm_user_role_v87, apm_system, "), binding("ns", "unrelated", fpB, "kibana_system"), binding("ns", "first", fpA, "apm_system,apm_user_role_v87,apm_system")},
			},
			wantKeys: []string{pkiMappingKey("ns", "first"), pkiMappingKey("ns", "second"), pkiMappingKey("ns", "unrelated")},
		},
		{
			name: "conflict: same fingerprint with different roles — both conflicting entries withheld",
			runs: [][]rolemapping.Request{
				{binding("ns", "first", fpA, "apm_system,apm_user_role_v87,apm_system"), binding("ns", "unrelated", fpB, "kibana_system"), binding("ns", "second", fpA, "superuser")},
				{binding("ns", "second", fpA, "superuser"), binding("ns", "unrelated", fpB, "kibana_system"), binding("ns", "first", fpA, "apm_system,apm_user_role_v87,apm_system")},
			},
			wantKeys:      []string{pkiMappingKey("ns", "unrelated")},
			wantConflicts: true,
		},
		{
			name: "conflict: second has same fingerprint but empty roles — invalid second excluded, first kept",
			runs: [][]rolemapping.Request{
				{binding("ns", "first", fpA, "apm_system,apm_user_role_v87,apm_system"), binding("ns", "unrelated", fpB, "kibana_system"), binding("ns", "second", fpA, " , ")},
				{binding("ns", "second", fpA, " , "), binding("ns", "unrelated", fpB, "kibana_system"), binding("ns", "first", fpA, "apm_system,apm_user_role_v87,apm_system")},
			},
			wantKeys: []string{pkiMappingKey("ns", "first"), pkiMappingKey("ns", "unrelated")},
		},
		{
			name: "conflict: second has different fingerprint — no conflict, all three entries written",
			runs: [][]rolemapping.Request{
				{binding("ns", "first", fpA, "apm_system,apm_user_role_v87,apm_system"), binding("ns", "unrelated", fpB, "kibana_system"), binding("ns", "second", "another-fp", "apm_system,apm_user_role_v87,apm_system")},
				{binding("ns", "second", "another-fp", "apm_system,apm_user_role_v87,apm_system"), binding("ns", "unrelated", fpB, "kibana_system"), binding("ns", "first", fpA, "apm_system,apm_user_role_v87,apm_system")},
			},
			wantKeys: []string{pkiMappingKey("ns", "first"), pkiMappingKey("ns", "second"), pkiMappingKey("ns", "unrelated")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg *commonv1.Config
			var conflictMessages map[string]string
			for _, bindings := range tt.runs {
				cfg, conflictMessages = buildPKIRoleMappings(t.Context(), bindings)
			}
			if tt.wantConflicts {
				require.NotEmpty(t, conflictMessages, "expected conflict messages but got none")
			} else {
				require.Empty(t, conflictMessages, "expected no conflict messages but got: %v", conflictMessages)
			}
			if tt.wantNil {
				require.Nil(t, cfg)
				return
			}
			require.NotNil(t, cfg)
			require.Len(t, cfg.Data, len(tt.wantKeys))
			for _, key := range tt.wantKeys {
				require.Contains(t, cfg.Data, key)
			}
			if tt.assertFn != nil {
				tt.assertFn(t, cfg)
			}
		})
	}
}

func TestBuildPKIRoleMappings_ConflictMessage(t *testing.T) {
	tests := []struct {
		name         string
		requests     []rolemapping.Request
		wantContains []string
	}{
		{
			name: "names the associated resources and the roles they request",
			requests: []rolemapping.Request{
				{SecretNamespace: "es-ns", SecretName: "kb-request", PublicKeyFingerprint: "fp", Roles: "kibana_system", ResourceNamespace: "kb-ns", ResourceName: "kb", AssociationName: "kb-es"},
				{SecretNamespace: "es-ns", SecretName: "agent-request", PublicKeyFingerprint: "fp", Roles: "superuser", ResourceNamespace: "agent-ns", ResourceName: "agent", AssociationName: "agent-es"},
			},
			wantContains: []string{
				"fingerprint fp",
				"agent-ns/agent (association agent-es) requests roles [superuser]",
				"kb-ns/kb (association kb-es) requests roles [kibana_system]",
				"clientCertificateSecretName",
			},
		},
		{
			name: "falls back to the request Secret when the associated resource is unknown",
			requests: []rolemapping.Request{
				{SecretNamespace: "es-ns", SecretName: "kb-request", PublicKeyFingerprint: "fp", Roles: "kibana_system"},
				{SecretNamespace: "es-ns", SecretName: "agent-request", PublicKeyFingerprint: "fp", Roles: "superuser"},
			},
			wantContains: []string{
				"es-ns/agent-request requests roles [superuser]",
				"es-ns/kb-request requests roles [kibana_system]",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, messages := buildPKIRoleMappings(t.Context(), tt.requests)
			require.Len(t, messages, 1)
			for _, want := range tt.wantContains {
				require.Contains(t, messages["fp"], want)
			}
		})
	}
}

func pkiRequestSecret(es esv1.Elasticsearch, name, roles, uid, assocName string) *corev1.Secret {
	return &corev1.Secret{
		Namespace: es.Namespace,
		Name:      name,
		Labels:    rolemapping.RequestLabels(es),
		Data: map[string][]byte{
			rolemapping.RolesField:           []byte(roles),
			rolemapping.ResourceUIDField:     []byte(uid),
			rolemapping.AssociationNameField: []byte(assocName),
		},
	}
}

// pkiRequestSecretWithCert is like pkiRequestSecret but also populates the public key fingerprint
// field from a generated test certificate.
func pkiRequestSecretWithCert(t *testing.T, es esv1.Elasticsearch, name, roles, uid, assocName string) *corev1.Secret {
	t.Helper()
	_, cert := pkiTestCertificate(t, name, nil)
	s := pkiRequestSecret(es, name, roles, uid, assocName)
	s.Data[rolemapping.PublicKeyFingerprintField] = []byte(certificates.PublicKeyFingerprint(cert))
	return s
}

func TestReconcilePKIRoleMappings(t *testing.T) {
	es := esv1.Elasticsearch{
		Namespace: "esNs",
		Name:      "esName",
		Spec: esv1.ElasticsearchSpec{
			HTTP: commonv1.HTTPConfigWithClientOptions{TLS: commonv1.TLSWithClientOptions{Client: commonv1.ClientOptions{Authentication: true}}},
			Auth: esv1.Auth{PKI: esv1.PKIRealm{Enabled: true}},
		},
	}
	fileSettingsSecretName := types.NamespacedName{
		Namespace: es.Namespace,
		Name:      esv1.FileSettingsSecretName(es.Name),
	}

	tests := []struct {
		name              string
		setup             func(t *testing.T) []client.Object
		wantNoSecret      bool // true when no file-settings Secret should exist after reconcile
		wantNoUpdate      bool // true when the Secret must not be updated (resource version must not change)
		wantUpdated       bool // true when the Secret must be updated (resource version must change)
		wantEmptyMappings bool
		wantMappingKeys   []string
		assertFn          func(t *testing.T, settings Settings)
		// checkFn runs after reconcile with the fake client, for assertions that require reading
		// resources other than the file-settings Secret (e.g. conflict annotations on request Secrets).
		checkFn func(t *testing.T, c k8s.Client)
		// mutateES changes the reconciled Elasticsearch, e.g. to disable PKI.
		mutateES func(es *esv1.Elasticsearch)
	}{
		{
			name: "create: file-settings Secret is created with PKI mappings",
			setup: func(t *testing.T) []client.Object {
				t.Helper()
				return []client.Object{
					&es,
					pkiRequestSecretWithCert(t, es, "kb-client-cert", "kibana_system", "uid-kb", "kb-es"),
					pkiRequestSecretWithCert(t, es, "fleet-client-cert", "fleet_server", "uid-fleet", "agent-es"),
				}
			},
			wantMappingKeys: []string{
				pkiMappingKey(es.Namespace, "kb-client-cert"),
				pkiMappingKey(es.Namespace, "fleet-client-cert"),
			},
			assertFn: func(t *testing.T, settings Settings) {
				t.Helper()
				entry, ok := settings.State.RoleMappings.Data[pkiMappingKey(es.Namespace, "kb-client-cert")].(map[string]any)
				require.True(t, ok)
				meta, ok := entry["metadata"].(map[string]any)
				require.True(t, ok)
				require.Equal(t, "eck", meta["managed_by"])
				require.Equal(t, "uid-kb", meta["eck_resource_uid"])
				require.Equal(t, "kb-es", meta["eck_association"])
			},
		},
		{
			name:        "update: existing file-settings Secret is updated idempotently",
			wantUpdated: true,
			setup: func(t *testing.T) []client.Object {
				t.Helper()
				// Pre-populate the file-settings Secret with a stale mapping for a secret that no longer exists.
				secret := fileSettingsSecretWithRoleMappings(t, es, map[string]any{
					pkiMappingKey(es.Namespace, "old-cert"): managedPKIEntry("uid-old"),
				})
				return []client.Object{
					&es,
					secret,
					pkiRequestSecretWithCert(t, es, "kb-client-cert", "kibana_system", "uid-kb", "kb-es"),
				}
			},
			wantMappingKeys: []string{
				pkiMappingKey(es.Namespace, "kb-client-cert"),
			},
		},
		{
			name: "cleanup: stale ECK PKI entries are removed when all requests are deleted",
			setup: func(t *testing.T) []client.Object {
				t.Helper()
				// Pre-populate the file-settings Secret with a PKI entry.
				secret := fileSettingsSecretWithRoleMappings(t, es, map[string]any{
					pkiMappingKey(es.Namespace, "old-cert"): managedPKIEntry("uid-old"),
				})
				// No PKI request Secrets — all have been deleted.
				return []client.Object{&es, secret}
			},
			wantEmptyMappings: true,
		},
		{
			name: "PKI disabled: ECK PKI entries are removed while requests exist",
			setup: func(t *testing.T) []client.Object {
				t.Helper()
				secret := fileSettingsSecretWithRoleMappings(t, es, map[string]any{
					pkiMappingKey(es.Namespace, "kb-client-cert"): managedPKIEntry("uid-kb"),
				})
				return []client.Object{
					&es,
					secret,
					pkiRequestSecretWithCert(t, es, "kb-client-cert", "kibana_system", "uid-kb", "kb-es"),
				}
			},
			mutateES:          func(es *esv1.Elasticsearch) { es.Spec.Auth.PKI.Enabled = false },
			wantEmptyMappings: true,
		},
		{
			name: "mTLS disabled: ECK PKI entries are removed while requests exist",
			setup: func(t *testing.T) []client.Object {
				t.Helper()
				secret := fileSettingsSecretWithRoleMappings(t, es, map[string]any{
					pkiMappingKey(es.Namespace, "kb-client-cert"): managedPKIEntry("uid-kb"),
				})
				return []client.Object{
					&es,
					secret,
					pkiRequestSecretWithCert(t, es, "kb-client-cert", "kibana_system", "uid-kb", "kb-es"),
				}
			},
			mutateES:          func(es *esv1.Elasticsearch) { es.Spec.HTTP.TLS.Client.Authentication = false },
			wantEmptyMappings: true,
		},
		{
			name: "legacy: user-defined eck_managed_pki_ mapping is preserved when there are no requests",
			setup: func(t *testing.T) []client.Object {
				t.Helper()
				// Written by a StackConfigPolicy created before the prefix was reserved: no ECK metadata.
				secret := fileSettingsSecretWithRoleMappings(t, es, map[string]any{
					commonv1.ECKPKIRoleMappingPrefix + "corporate": userEntry("superuser"),
				})
				return []client.Object{&es, secret}
			},
			wantNoUpdate:    true,
			wantMappingKeys: []string{commonv1.ECKPKIRoleMappingPrefix + "corporate"},
		},
		{
			name:        "legacy: only ECK-generated entries are replaced next to a user-defined prefixed mapping",
			wantUpdated: true,
			setup: func(t *testing.T) []client.Object {
				t.Helper()
				secret := fileSettingsSecretWithRoleMappings(t, es, map[string]any{
					commonv1.ECKPKIRoleMappingPrefix + "corporate": userEntry("superuser"),
					"custom_mapping":                        userEntry("viewer"),
					pkiMappingKey(es.Namespace, "old-cert"): managedPKIEntry("uid-old"),
				})
				return []client.Object{
					&es,
					secret,
					pkiRequestSecretWithCert(t, es, "kb-client-cert", "kibana_system", "uid-kb", "kb-es"),
				}
			},
			wantMappingKeys: []string{
				commonv1.ECKPKIRoleMappingPrefix + "corporate",
				"custom_mapping",
				pkiMappingKey(es.Namespace, "kb-client-cert"),
			},
			assertFn: func(t *testing.T, settings Settings) {
				t.Helper()
				require.Equal(t, userEntry("superuser"), settings.State.RoleMappings.Data[commonv1.ECKPKIRoleMappingPrefix+"corporate"])
			},
		},
		{
			name: "collision: generated mapping is withheld and the user-defined mapping with the same name is untouched",
			setup: func(t *testing.T) []client.Object {
				t.Helper()
				secret := fileSettingsSecretWithRoleMappings(t, es, map[string]any{
					pkiMappingKey(es.Namespace, "kb-client-cert"): userEntry("superuser"),
				})
				return []client.Object{
					&es,
					secret,
					pkiRequestSecretWithCert(t, es, "kb-client-cert", "kibana_system", "uid-kb", "kb-es"),
				}
			},
			wantNoUpdate:    true,
			wantMappingKeys: []string{pkiMappingKey(es.Namespace, "kb-client-cert")},
			assertFn: func(t *testing.T, settings Settings) {
				t.Helper()
				require.Equal(t, userEntry("superuser"), settings.State.RoleMappings.Data[pkiMappingKey(es.Namespace, "kb-client-cert")])
			},
			checkFn: func(t *testing.T, c k8s.Client) {
				t.Helper()
				var s corev1.Secret
				require.NoError(t, c.Get(t.Context(), types.NamespacedName{Namespace: es.Namespace, Name: "kb-client-cert"}, &s))
				require.Contains(t, s.Annotations[rolemapping.PKIConflictAnnotation], "StackConfigPolicy")
			},
		},
		{
			name:        "collision resolved: generated mapping is written and the annotation cleared once the user mapping is gone",
			wantUpdated: true,
			setup: func(t *testing.T) []client.Object {
				t.Helper()
				secret := fileSettingsSecretWithRoleMappings(t, es, map[string]any{"custom_mapping": userEntry("viewer")})
				req := pkiRequestSecretWithCert(t, es, "kb-client-cert", "kibana_system", "uid-kb", "kb-es")
				req.Annotations = map[string]string{rolemapping.PKIConflictAnnotation: "previous collision"}
				return []client.Object{&es, secret, req}
			},
			wantMappingKeys: []string{"custom_mapping", pkiMappingKey(es.Namespace, "kb-client-cert")},
			checkFn: func(t *testing.T, c k8s.Client) {
				t.Helper()
				var s corev1.Secret
				require.NoError(t, c.Get(t.Context(), types.NamespacedName{Namespace: es.Namespace, Name: "kb-client-cert"}, &s))
				require.Empty(t, s.Annotations[rolemapping.PKIConflictAnnotation])
			},
		},
		{
			name: "no-op: no requests and no existing ECK PKI entries skips Secret creation",
			setup: func(t *testing.T) []client.Object {
				t.Helper()
				return []client.Object{&es}
			},
			wantNoSecret: true,
		},
		{
			name: "no-op: existing entries match incoming requests, no update",
			setup: func(t *testing.T) []client.Object {
				t.Helper()
				req := pkiRequestSecret(es, "kb-client-cert", "kibana_system", "uid-kb", "kb-es")
				_, cert := pkiTestCertificate(t, "kb-client-cert", nil)
				req.Data[rolemapping.PublicKeyFingerprintField] = []byte(certificates.PublicKeyFingerprint(cert))
				// First reconcile populates the file-settings Secret with the correct entries.
				setupClient := k8s.NewFakeClient(&es, req)
				require.NoError(t, ReconcilePKIRoleMappings(t.Context(), setupClient, es))
				var secret corev1.Secret
				require.NoError(t, setupClient.Get(t.Context(), fileSettingsSecretName, &secret))
				return []client.Object{&es, &secret, req}
			},
			wantNoUpdate:    true,
			wantMappingKeys: []string{pkiMappingKey(es.Namespace, "kb-client-cert")},
		},
		{
			name:        "conflict: same fingerprint with different roles withholds all affected mappings",
			wantUpdated: true,
			setup: func(t *testing.T) []client.Object {
				t.Helper()
				_, cert := pkiTestCertificate(t, "shared", nil)
				fp := []byte(certificates.PublicKeyFingerprint(cert))
				first := pkiRequestSecret(es, "first", "kibana_system", "uid1", "kb-es")
				first.Data[rolemapping.PublicKeyFingerprintField] = fp
				second := pkiRequestSecret(es, "second", "superuser", "uid2", "other-es")
				second.Data[rolemapping.PublicKeyFingerprintField] = fp
				// Pre-populate the file-settings Secret with the mapping for "first" (pre-conflict state).
				setupClient := k8s.NewFakeClient(&es, first)
				require.NoError(t, ReconcilePKIRoleMappings(t.Context(), setupClient, es))
				var fsSecret corev1.Secret
				require.NoError(t, setupClient.Get(t.Context(), fileSettingsSecretName, &fsSecret))
				return []client.Object{&es, &fsSecret, first, second}
			},
			wantEmptyMappings: true,
			checkFn: func(t *testing.T, c k8s.Client) {
				t.Helper()
				for _, name := range []string{"first", "second"} {
					var s corev1.Secret
					require.NoError(t, c.Get(t.Context(), types.NamespacedName{Namespace: es.Namespace, Name: name}, &s))
					require.NotEmpty(t, s.Annotations[rolemapping.PKIConflictAnnotation],
						"request Secret %q must have conflict annotation", name)
				}
			},
		},
		{
			name:        "conflict resolved: mapping restored when conflicting request is removed",
			wantUpdated: true,
			setup: func(t *testing.T) []client.Object {
				t.Helper()
				_, cert := pkiTestCertificate(t, "shared", nil)
				fp := []byte(certificates.PublicKeyFingerprint(cert))
				first := pkiRequestSecret(es, "first", "kibana_system", "uid1", "kb-es")
				first.Data[rolemapping.PublicKeyFingerprintField] = fp
				second := pkiRequestSecret(es, "second", "superuser", "uid2", "other-es")
				second.Data[rolemapping.PublicKeyFingerprintField] = fp
				// Phase 1: only first — Secret created with mapping.
				setupClient := k8s.NewFakeClient(&es, first)
				require.NoError(t, ReconcilePKIRoleMappings(t.Context(), setupClient, es))
				// Phase 2: second added — role_mappings emptied, annotations written.
				require.NoError(t, setupClient.Create(t.Context(), second))
				require.NoError(t, ReconcilePKIRoleMappings(t.Context(), setupClient, es))
				var fsSecret corev1.Secret
				require.NoError(t, setupClient.Get(t.Context(), fileSettingsSecretName, &fsSecret))
				// Read first back so the final client carries the annotation written in phase 2.
				var firstAnnotated corev1.Secret
				require.NoError(t, setupClient.Get(t.Context(), types.NamespacedName{Namespace: es.Namespace, Name: "first"}, &firstAnnotated))
				require.NotEmpty(t, firstAnnotated.Annotations[rolemapping.PKIConflictAnnotation],
					"precondition: first must carry the conflict annotation after phase 2")
				return []client.Object{&es, &fsSecret, &firstAnnotated} // second removed; conflict resolved
			},
			wantMappingKeys: []string{pkiMappingKey(es.Namespace, "first")},
			checkFn: func(t *testing.T, c k8s.Client) {
				t.Helper()
				var s corev1.Secret
				require.NoError(t, c.Get(t.Context(), types.NamespacedName{Namespace: es.Namespace, Name: "first"}, &s))
				require.Empty(t, s.Annotations[rolemapping.PKIConflictAnnotation],
					"conflict annotation must be cleared once the conflict is resolved")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.False(t, tt.wantNoUpdate && tt.wantUpdated, "wantNoUpdate and wantUpdated are mutually exclusive")
			var objs []client.Object
			if tt.setup != nil {
				objs = tt.setup(t)
			}
			fakeClient := k8s.NewFakeClient(objs...)

			var rvBefore string
			if tt.wantNoUpdate || tt.wantUpdated {
				var s corev1.Secret
				require.NoError(t, fakeClient.Get(context.Background(), fileSettingsSecretName, &s))
				rvBefore = s.ResourceVersion
			}

			reconciledES := *es.DeepCopy()
			if tt.mutateES != nil {
				tt.mutateES(&reconciledES)
			}
			require.NoError(t, ReconcilePKIRoleMappings(context.Background(), fakeClient, reconciledES))

			var secret corev1.Secret
			getErr := fakeClient.Get(context.Background(), fileSettingsSecretName, &secret)
			if tt.wantNoSecret {
				require.True(t, apierrors.IsNotFound(getErr), "expected no file-settings Secret, got: %v", getErr)
				return
			}
			require.NoError(t, getErr)
			if tt.wantNoUpdate {
				require.Equal(t, rvBefore, secret.ResourceVersion, "file-settings Secret must not be updated when entries are unchanged")
			}
			if tt.wantUpdated {
				require.NotEqual(t, rvBefore, secret.ResourceVersion, "file-settings Secret must be updated when entries change")
			}

			settings := parseSettings(t, secret)
			if tt.wantEmptyMappings {
				// role_mappings must be written as {} (not omitted) so that Elasticsearch removes stale mappings.
				require.NotNil(t, settings.State.RoleMappings)
				require.Empty(t, settings.State.RoleMappings.Data)
			} else {
				require.NotNil(t, settings.State.RoleMappings)
				for _, key := range tt.wantMappingKeys {
					require.Contains(t, settings.State.RoleMappings.Data, key)
				}
				// Stale entries from previous reconciles must not be present.
				require.Len(t, settings.State.RoleMappings.Data, len(tt.wantMappingKeys))
				if tt.assertFn != nil {
					tt.assertFn(t, settings)
				}
			}
			if tt.checkFn != nil {
				tt.checkFn(t, fakeClient)
			}
		})
	}

	// This case needs shared state between setup and assertion (resource version captured before
	// the reconcile under test, compared after), so it lives outside the table.
	t.Run("conflict stable: skip branch taken when annotation is already correct", func(t *testing.T) {
		_, cert := pkiTestCertificate(t, "shared", nil)
		fp := []byte(certificates.PublicKeyFingerprint(cert))
		first := pkiRequestSecret(es, "first", "kibana_system", "uid1", "kb-es")
		first.Data[rolemapping.PublicKeyFingerprintField] = fp
		second := pkiRequestSecret(es, "second", "superuser", "uid2", "other-es")
		second.Data[rolemapping.PublicKeyFingerprintField] = fp

		// Phase 1: establish conflict so annotations are written onto both Secrets.
		setupClient := k8s.NewFakeClient(&es, first, second)
		require.NoError(t, ReconcilePKIRoleMappings(t.Context(), setupClient, es))

		var firstAnnotated, secondAnnotated corev1.Secret
		require.NoError(t, setupClient.Get(t.Context(), types.NamespacedName{Namespace: es.Namespace, Name: "first"}, &firstAnnotated))
		require.NoError(t, setupClient.Get(t.Context(), types.NamespacedName{Namespace: es.Namespace, Name: "second"}, &secondAnnotated))
		require.NotEmpty(t, firstAnnotated.Annotations[rolemapping.PKIConflictAnnotation], "precondition")
		require.NotEmpty(t, secondAnnotated.Annotations[rolemapping.PKIConflictAnnotation], "precondition")

		// Phase 2: run reconcile with annotated Secrets in the client.
		// ConflictAnnotation == desiredMsg for both → skip branch must be taken, no patch issued.
		// No file-settings Secret is needed: a pure conflict produces nil pkiMappings, which the
		// unchanged check catches before a write is attempted.
		rvFirst := firstAnnotated.ResourceVersion
		rvSecond := secondAnnotated.ResourceVersion
		fakeClient := k8s.NewFakeClient(&es, &firstAnnotated, &secondAnnotated)
		require.NoError(t, ReconcilePKIRoleMappings(t.Context(), fakeClient, es))

		var s corev1.Secret
		require.NoError(t, fakeClient.Get(t.Context(), types.NamespacedName{Namespace: es.Namespace, Name: "first"}, &s))
		require.Equal(t, rvFirst, s.ResourceVersion, "no patch expected: annotation already correct on first")
		require.NoError(t, fakeClient.Get(t.Context(), types.NamespacedName{Namespace: es.Namespace, Name: "second"}, &s))
		require.Equal(t, rvSecond, s.ResourceVersion, "no patch expected: annotation already correct on second")
	})
}

func TestPatchConflictAnnotations_PatchFailureIsReturned(t *testing.T) {
	patchErr := fmt.Errorf("injected patch failure")
	c := k8s.NewFakeClientBuilder().
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(_ context.Context, _ client.WithWatch, _ client.Object, _ client.Patch, _ ...client.PatchOption) error {
				return patchErr
			},
		}).Build()

	req := rolemapping.Request{
		SecretNamespace:      "ns",
		SecretName:           "kb-client-cert",
		PublicKeyFingerprint: "some-fp",
		ConflictAnnotation:   "", // differs from desiredMsg, so patch is attempted
	}
	err := patchConflictAnnotations(t.Context(), c, []rolemapping.Request{req}, map[string]string{"some-fp": "conflict message"}, nil)
	require.Error(t, err)
	require.ErrorIs(t, err, patchErr)
	require.ErrorContains(t, err, "ns/kb-client-cert")
}

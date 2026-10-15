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
	"encoding/json"
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
	essettings "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
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

// requestEvaluation returns the evaluation stored on the request Secret name, and whether there is one.
func requestEvaluation(t *testing.T, c k8s.Client, namespace, name string) (rolemapping.Evaluation, bool) {
	t.Helper()
	var s corev1.Secret
	require.NoError(t, c.Get(t.Context(), types.NamespacedName{Namespace: namespace, Name: name}, &s))
	return rolemapping.ParseEvaluation(s.Annotations[rolemapping.PKIEvaluationAnnotation])
}

// evaluationAnnotation returns the PKIEvaluationAnnotation value of evaluation.
func evaluationAnnotation(t *testing.T, evaluation rolemapping.Evaluation) string {
	t.Helper()
	value, err := json.Marshal(evaluation)
	require.NoError(t, err)
	return string(value)
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

// reconcileAndEvaluate runs ReconcilePKIRoleMappings with the ECK-managed PKI realm configured and applies the
// evaluations, as the Elasticsearch driver does once the realm runs on all the nodes.
func reconcileAndEvaluate(t *testing.T, c k8s.Client, es esv1.Elasticsearch) {
	t.Helper()
	evaluations, err := ReconcilePKIRoleMappings(t.Context(), c, es, true)
	require.NoError(t, err)
	require.NoError(t, evaluations.Apply(t.Context(), c))
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
		// clientAuthenticationIneffective simulates mTLS being enabled in the spec but not effectively required, e.g.
		// without enterprise features or with a client_authentication override in the user or StackConfigPolicy config.
		clientAuthenticationIneffective bool
		// realmNotRunning leaves the conflict-free evaluations unapplied, as when the realm is not on all the nodes.
		realmNotRunning bool
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
			checkFn: func(t *testing.T, c k8s.Client) {
				t.Helper()
				for name, roles := range map[string]string{"kb-client-cert": "kibana_system", "fleet-client-cert": "fleet_server"} {
					var req corev1.Secret
					require.NoError(t, c.Get(t.Context(), types.NamespacedName{Namespace: es.Namespace, Name: name}, &req))
					evaluation, ok := requestEvaluation(t, c, es.Namespace, name)
					require.True(t, ok, "request Secret %q must be evaluated", name)
					require.True(t, evaluation.Matches(string(req.Data[rolemapping.PublicKeyFingerprintField]), roles))
					require.Empty(t, evaluation.Conflict)
				}
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
			name: "PKI disabled: ECK PKI entries and evaluations are removed while requests exist",
			setup: func(t *testing.T) []client.Object {
				t.Helper()
				secret := fileSettingsSecretWithRoleMappings(t, es, map[string]any{
					pkiMappingKey(es.Namespace, "kb-client-cert"): managedPKIEntry("uid-kb"),
				})
				req := pkiRequestSecretWithCert(t, es, "kb-client-cert", "kibana_system", "uid-kb", "kb-es")
				req.Annotations = map[string]string{rolemapping.PKIEvaluationAnnotation: evaluationAnnotation(t, rolemapping.Evaluation{
					PublicKeyFingerprint: string(req.Data[rolemapping.PublicKeyFingerprintField]),
					Roles:                "kibana_system",
				})}
				return []client.Object{&es, secret, req}
			},
			mutateES:          func(es *esv1.Elasticsearch) { es.Spec.Auth.PKI.Enabled = false },
			wantEmptyMappings: true,
			checkFn: func(t *testing.T, c k8s.Client) {
				t.Helper()
				_, ok := requestEvaluation(t, c, es.Namespace, "kb-client-cert")
				require.False(t, ok, "the evaluation must be removed when PKI is disabled")
			},
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
			name: "mTLS ineffective: ECK PKI entries and evaluations are removed while the spec enables PKI and mTLS",
			setup: func(t *testing.T) []client.Object {
				t.Helper()
				secret := fileSettingsSecretWithRoleMappings(t, es, map[string]any{
					pkiMappingKey(es.Namespace, "kb-client-cert"): managedPKIEntry("uid-kb"),
				})
				req := pkiRequestSecretWithCert(t, es, "kb-client-cert", "kibana_system", "uid-kb", "kb-es")
				req.Annotations = map[string]string{rolemapping.PKIEvaluationAnnotation: evaluationAnnotation(t, rolemapping.Evaluation{
					PublicKeyFingerprint: string(req.Data[rolemapping.PublicKeyFingerprintField]),
					Roles:                "kibana_system",
				})}
				return []client.Object{&es, secret, req}
			},
			clientAuthenticationIneffective: true,
			wantEmptyMappings:               true,
			checkFn: func(t *testing.T, c k8s.Client) {
				t.Helper()
				_, ok := requestEvaluation(t, c, es.Namespace, "kb-client-cert")
				require.False(t, ok, "the evaluation must be removed when mTLS is not effectively required")
			},
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
				evaluation, ok := requestEvaluation(t, c, es.Namespace, "kb-client-cert")
				require.True(t, ok)
				require.Contains(t, evaluation.Conflict, "StackConfigPolicy")
			},
		},
		{
			name:        "collision resolved: generated mapping is written and the annotation cleared once the user mapping is gone",
			wantUpdated: true,
			setup: func(t *testing.T) []client.Object {
				t.Helper()
				secret := fileSettingsSecretWithRoleMappings(t, es, map[string]any{"custom_mapping": userEntry("viewer")})
				req := pkiRequestSecretWithCert(t, es, "kb-client-cert", "kibana_system", "uid-kb", "kb-es")
				req.Annotations = map[string]string{rolemapping.PKIEvaluationAnnotation: evaluationAnnotation(t, rolemapping.Evaluation{
					PublicKeyFingerprint: string(req.Data[rolemapping.PublicKeyFingerprintField]),
					Roles:                "kibana_system",
					Conflict:             "previous collision",
				})}
				return []client.Object{&es, secret, req}
			},
			wantMappingKeys: []string{"custom_mapping", pkiMappingKey(es.Namespace, "kb-client-cert")},
			checkFn: func(t *testing.T, c k8s.Client) {
				t.Helper()
				evaluation, ok := requestEvaluation(t, c, es.Namespace, "kb-client-cert")
				require.True(t, ok)
				require.Empty(t, evaluation.Conflict)
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
				reconcileAndEvaluate(t, setupClient, es)
				var secret corev1.Secret
				require.NoError(t, setupClient.Get(t.Context(), fileSettingsSecretName, &secret))
				return []client.Object{&es, &secret, req}
			},
			wantNoUpdate:    true,
			wantMappingKeys: []string{pkiMappingKey(es.Namespace, "kb-client-cert")},
		},
		{
			name: "realm configured but not running on all nodes: mappings are written and requests are not evaluated",
			setup: func(t *testing.T) []client.Object {
				t.Helper()
				return []client.Object{&es, pkiRequestSecretWithCert(t, es, "kb-client-cert", "kibana_system", "uid-kb", "kb-es")}
			},
			realmNotRunning: true,
			wantMappingKeys: []string{pkiMappingKey(es.Namespace, "kb-client-cert")},
			checkFn: func(t *testing.T, c k8s.Client) {
				t.Helper()
				_, ok := requestEvaluation(t, c, es.Namespace, "kb-client-cert")
				require.False(t, ok, "the request must not be evaluated before the realm runs on all the nodes")
			},
		},
		{
			name: "realm running on all nodes after the mappings were written: requests are evaluated without a new write",
			setup: func(t *testing.T) []client.Object {
				t.Helper()
				req := pkiRequestSecretWithCert(t, es, "kb-client-cert", "kibana_system", "uid-kb", "kb-es")
				setupClient := k8s.NewFakeClient(&es, req)
				// the evaluations are not applied while the realm does not run on all the nodes
				_, err := ReconcilePKIRoleMappings(t.Context(), setupClient, es, true)
				require.NoError(t, err)
				var secret, request corev1.Secret
				require.NoError(t, setupClient.Get(t.Context(), fileSettingsSecretName, &secret))
				require.NoError(t, setupClient.Get(t.Context(), types.NamespacedName{Namespace: es.Namespace, Name: "kb-client-cert"}, &request))
				_, evaluated := requestEvaluation(t, setupClient, es.Namespace, "kb-client-cert")
				require.False(t, evaluated, "precondition: the request is not evaluated while the realm is not running")
				return []client.Object{&es, &secret, &request}
			},
			wantNoUpdate:    true,
			wantMappingKeys: []string{pkiMappingKey(es.Namespace, "kb-client-cert")},
			checkFn: func(t *testing.T, c k8s.Client) {
				t.Helper()
				evaluation, ok := requestEvaluation(t, c, es.Namespace, "kb-client-cert")
				require.True(t, ok)
				require.Empty(t, evaluation.Conflict)
			},
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
				reconcileAndEvaluate(t, setupClient, es)
				var fsSecret corev1.Secret
				require.NoError(t, setupClient.Get(t.Context(), fileSettingsSecretName, &fsSecret))
				return []client.Object{&es, &fsSecret, first, second}
			},
			wantEmptyMappings: true,
			checkFn: func(t *testing.T, c k8s.Client) {
				t.Helper()
				for _, name := range []string{"first", "second"} {
					evaluation, ok := requestEvaluation(t, c, es.Namespace, name)
					require.True(t, ok, "request Secret %q must be evaluated", name)
					require.NotEmpty(t, evaluation.Conflict, "request Secret %q must have a conflict", name)
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
				reconcileAndEvaluate(t, setupClient, es)
				// Phase 2: second added — role_mappings emptied, annotations written.
				require.NoError(t, setupClient.Create(t.Context(), second))
				reconcileAndEvaluate(t, setupClient, es)
				var fsSecret corev1.Secret
				require.NoError(t, setupClient.Get(t.Context(), fileSettingsSecretName, &fsSecret))
				// Read first back so the final client carries the annotation written in phase 2.
				var firstAnnotated corev1.Secret
				require.NoError(t, setupClient.Get(t.Context(), types.NamespacedName{Namespace: es.Namespace, Name: "first"}, &firstAnnotated))
				evaluation, ok := rolemapping.ParseEvaluation(firstAnnotated.Annotations[rolemapping.PKIEvaluationAnnotation])
				require.True(t, ok, "precondition: first must be evaluated after phase 2")
				require.NotEmpty(t, evaluation.Conflict, "precondition: first must have a conflict after phase 2")
				return []client.Object{&es, &fsSecret, &firstAnnotated} // second removed; conflict resolved
			},
			wantMappingKeys: []string{pkiMappingKey(es.Namespace, "first")},
			checkFn: func(t *testing.T, c k8s.Client) {
				t.Helper()
				evaluation, ok := requestEvaluation(t, c, es.Namespace, "first")
				require.True(t, ok)
				require.Empty(t, evaluation.Conflict, "the conflict must be cleared once the conflict is resolved")
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
			clientAuthenticationRequired := reconciledES.Spec.HTTP.TLS.Client.Authentication && !tt.clientAuthenticationIneffective
			pkiRealmConfigured := essettings.PKIRealmConfigured(reconciledES.Spec.Auth.PKI.Enabled, clientAuthenticationRequired, commonv1.PKIAuthMinVersion)
			evaluations, err := ReconcilePKIRoleMappings(context.Background(), fakeClient, reconciledES, pkiRealmConfigured)
			require.NoError(t, err)
			if !tt.realmNotRunning {
				require.NoError(t, evaluations.Apply(context.Background(), fakeClient))
			}

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
		reconcileAndEvaluate(t, setupClient, es)

		var firstAnnotated, secondAnnotated corev1.Secret
		require.NoError(t, setupClient.Get(t.Context(), types.NamespacedName{Namespace: es.Namespace, Name: "first"}, &firstAnnotated))
		require.NoError(t, setupClient.Get(t.Context(), types.NamespacedName{Namespace: es.Namespace, Name: "second"}, &secondAnnotated))
		require.NotEmpty(t, firstAnnotated.Annotations[rolemapping.PKIEvaluationAnnotation], "precondition")
		require.NotEmpty(t, secondAnnotated.Annotations[rolemapping.PKIEvaluationAnnotation], "precondition")

		// Phase 2: run reconcile with annotated Secrets in the client.
		// The current evaluations equal the desired ones for both → skip branch must be taken, no patch issued.
		// No file-settings Secret is needed: a pure conflict produces nil pkiMappings, which the
		// unchanged check catches before a write is attempted.
		rvFirst := firstAnnotated.ResourceVersion
		rvSecond := secondAnnotated.ResourceVersion
		fakeClient := k8s.NewFakeClient(&es, &firstAnnotated, &secondAnnotated)
		reconcileAndEvaluate(t, fakeClient, es)

		var s corev1.Secret
		require.NoError(t, fakeClient.Get(t.Context(), types.NamespacedName{Namespace: es.Namespace, Name: "first"}, &s))
		require.Equal(t, rvFirst, s.ResourceVersion, "no patch expected: annotation already correct on first")
		require.NoError(t, fakeClient.Get(t.Context(), types.NamespacedName{Namespace: es.Namespace, Name: "second"}, &s))
		require.Equal(t, rvSecond, s.ResourceVersion, "no patch expected: annotation already correct on second")
	})
}

func TestPatchEvaluationAnnotations_PatchFailureIsReturned(t *testing.T) {
	patchErr := fmt.Errorf("injected patch failure")
	c := k8s.NewFakeClientBuilder().
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(_ context.Context, _ client.WithWatch, _ client.Object, _ client.Patch, _ ...client.PatchOption) error {
				return patchErr
			},
		}).Build()

	err := patchEvaluationAnnotations(t.Context(), c, []evaluationUpdate{{namespace: "ns", name: "kb-client-cert", value: "evaluation"}})
	require.Error(t, err)
	require.ErrorIs(t, err, patchErr)
	require.ErrorContains(t, err, "ns/kb-client-cert")
}

// TestReconcilePKIRoleMappings_Evaluations checks which evaluation annotations ReconcilePKIRoleMappings updates right
// away, and which it returns to be applied once the ECK-managed PKI realm runs on all the nodes: only the
// conflict-free evaluations, which let an association switch to PKI, wait.
func TestReconcilePKIRoleMappings_Evaluations(t *testing.T) {
	es := esv1.Elasticsearch{Namespace: "ns", Name: "es"}
	const (
		// annotation states of a request Secret
		none      = "no annotation"
		unchanged = "annotation unchanged"
		evaluated = "conflict-free evaluation"
		conflict  = "evaluation with a conflict"
	)
	// annotated returns a copy of request with the evaluation annotation of its content, with the given conflict.
	annotated := func(t *testing.T, request *corev1.Secret, conflict string) *corev1.Secret {
		t.Helper()
		request = request.DeepCopy()
		request.Annotations = map[string]string{rolemapping.PKIEvaluationAnnotation: evaluationAnnotation(t, rolemapping.Evaluation{
			PublicKeyFingerprint: string(request.Data[rolemapping.PublicKeyFingerprintField]),
			Roles:                string(request.Data[rolemapping.RolesField]),
			Conflict:             conflict,
		})}
		return request
	}
	// sharedFingerprint returns two requests with the same public key fingerprint asking for different roles.
	sharedFingerprint := func(t *testing.T) (*corev1.Secret, *corev1.Secret) {
		t.Helper()
		first := pkiRequestSecretWithCert(t, es, "first", "kibana_system", "uid1", "kb-es")
		second := pkiRequestSecret(es, "second", "superuser", "uid2", "other-es")
		second.Data[rolemapping.PublicKeyFingerprintField] = first.Data[rolemapping.PublicKeyFingerprintField]
		return first, second
	}
	tests := []struct {
		name               string
		pkiRealmConfigured bool
		setup              func(t *testing.T) []client.Object
		wantPending        bool
		// wantBefore and wantAfter are the annotation states of the request Secrets by name, before and after the
		// returned evaluations are applied.
		wantBefore map[string]string
		wantAfter  map[string]string
	}{
		{
			name:               "new request: pending, annotated once applied",
			pkiRealmConfigured: true,
			setup: func(t *testing.T) []client.Object {
				t.Helper()
				return []client.Object{pkiRequestSecretWithCert(t, es, "first", "kibana_system", "uid1", "kb-es")}
			},
			wantPending: true,
			wantBefore:  map[string]string{"first": none},
			wantAfter:   map[string]string{"first": evaluated},
		},
		{
			name:               "request changed since its evaluation: pending, previous annotation kept until applied",
			pkiRealmConfigured: true,
			setup: func(t *testing.T) []client.Object {
				t.Helper()
				request := pkiRequestSecretWithCert(t, es, "first", "kibana_system", "uid1", "kb-es")
				request.Annotations = map[string]string{rolemapping.PKIEvaluationAnnotation: "previous"}
				return []client.Object{request}
			},
			wantPending: true,
			wantBefore:  map[string]string{"first": unchanged},
			wantAfter:   map[string]string{"first": evaluated},
		},
		{
			name:               "request already evaluated: not pending",
			pkiRealmConfigured: true,
			setup: func(t *testing.T) []client.Object {
				t.Helper()
				return []client.Object{annotated(t, pkiRequestSecretWithCert(t, es, "first", "kibana_system", "uid1", "kb-es"), "")}
			},
			wantPending: false,
			wantBefore:  map[string]string{"first": unchanged},
			wantAfter:   map[string]string{"first": unchanged},
		},
		{
			name:               "fingerprint conflict appears on evaluated requests: conflict annotated right away, not pending",
			pkiRealmConfigured: true,
			setup: func(t *testing.T) []client.Object {
				t.Helper()
				first, second := sharedFingerprint(t)
				return []client.Object{annotated(t, first, ""), annotated(t, second, "")}
			},
			wantPending: false,
			wantBefore:  map[string]string{"first": conflict, "second": conflict},
			wantAfter:   map[string]string{"first": conflict, "second": conflict},
		},
		{
			name:               "fingerprint conflict on new requests: conflict annotated right away, not pending",
			pkiRealmConfigured: true,
			setup: func(t *testing.T) []client.Object {
				t.Helper()
				first, second := sharedFingerprint(t)
				return []client.Object{first, second}
			},
			wantPending: false,
			wantBefore:  map[string]string{"first": conflict, "second": conflict},
			wantAfter:   map[string]string{"first": conflict, "second": conflict},
		},
		{
			name:               "fingerprint conflict disappears: pending, conflict kept until applied",
			pkiRealmConfigured: true,
			setup: func(t *testing.T) []client.Object {
				t.Helper()
				first, _ := sharedFingerprint(t)
				return []client.Object{annotated(t, first, "previous conflict")}
			},
			wantPending: true,
			wantBefore:  map[string]string{"first": conflict},
			wantAfter:   map[string]string{"first": evaluated},
		},
		{
			name:               "name collision appears on an evaluated request: conflict annotated right away, not pending",
			pkiRealmConfigured: true,
			setup: func(t *testing.T) []client.Object {
				t.Helper()
				return []client.Object{
					annotated(t, pkiRequestSecretWithCert(t, es, "first", "kibana_system", "uid1", "kb-es"), ""),
					fileSettingsSecretWithRoleMappings(t, es, map[string]any{pkiMappingKey(es.Namespace, "first"): userEntry("superuser")}),
				}
			},
			wantPending: false,
			wantBefore:  map[string]string{"first": conflict},
			wantAfter:   map[string]string{"first": conflict},
		},
		{
			name:               "name collision disappears: pending, conflict kept until applied",
			pkiRealmConfigured: true,
			setup: func(t *testing.T) []client.Object {
				t.Helper()
				return []client.Object{annotated(t, pkiRequestSecretWithCert(t, es, "first", "kibana_system", "uid1", "kb-es"), "previous collision")}
			},
			wantPending: true,
			wantBefore:  map[string]string{"first": conflict},
			wantAfter:   map[string]string{"first": evaluated},
		},
		{
			name:               "realm not configured: evaluation and conflict removed right away, not pending",
			pkiRealmConfigured: false,
			setup: func(t *testing.T) []client.Object {
				t.Helper()
				first, second := sharedFingerprint(t)
				return []client.Object{annotated(t, first, ""), annotated(t, second, "previous conflict")}
			},
			wantPending: false,
			wantBefore:  map[string]string{"first": none, "second": none},
			wantAfter:   map[string]string{"first": none, "second": none},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := append([]client.Object{&es}, tt.setup(t)...)
			initial := make(map[string]string)
			for _, obj := range objs {
				if secret, ok := obj.(*corev1.Secret); ok {
					initial[secret.Name] = secret.Annotations[rolemapping.PKIEvaluationAnnotation]
				}
			}
			c := k8s.NewFakeClient(objs...)
			assertStates := func(t *testing.T, want map[string]string) {
				t.Helper()
				for name, state := range want {
					var request corev1.Secret
					require.NoError(t, c.Get(t.Context(), types.NamespacedName{Namespace: es.Namespace, Name: name}, &request))
					raw := request.Annotations[rolemapping.PKIEvaluationAnnotation]
					switch state {
					case none:
						require.Empty(t, raw, name)
					case unchanged:
						require.Equal(t, initial[name], raw, name)
					case evaluated, conflict:
						evaluation, ok := rolemapping.ParseEvaluation(raw)
						require.True(t, ok, name)
						require.True(t, evaluation.Matches(string(request.Data[rolemapping.PublicKeyFingerprintField]), string(request.Data[rolemapping.RolesField])), name)
						require.Equal(t, state == conflict, evaluation.Conflict != "", "%s: %s", name, evaluation.Conflict)
					}
				}
			}

			evaluations, err := ReconcilePKIRoleMappings(t.Context(), c, es, tt.pkiRealmConfigured)
			require.NoError(t, err)
			require.Equal(t, tt.wantPending, evaluations.Pending())
			assertStates(t, tt.wantBefore)

			require.NoError(t, evaluations.Apply(t.Context(), c))
			assertStates(t, tt.wantAfter)
			// Nothing is pending once the evaluations are applied.
			evaluations, err = ReconcilePKIRoleMappings(t.Context(), c, es, tt.pkiRealmConfigured)
			require.NoError(t, err)
			require.False(t, evaluations.Pending())
		})
	}
}

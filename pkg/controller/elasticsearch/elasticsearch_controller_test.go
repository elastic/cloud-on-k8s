// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.
package elasticsearch

import (
	"context"
	"testing"

	"github.com/sethvargo/go-password/password"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	commonpassword "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/password"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/hints"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/test"
)

// newTestReconciler returns a ReconcileElasticsearch struct, allowing the internal k8s client to
// contain certain runtime objects.
func newTestReconciler(objects ...client.Object) *ReconcileElasticsearch {
	r := &ReconcileElasticsearch{
		Client:   k8s.NewFakeClient(objects...),
		recorder: record.NewFakeRecorder(100),
	}
	return r
}

// esBuilder allows for a cleaner way to build a testable elasticsearch spec,
// minimizing repetition.
type esBuilder struct {
	es *esv1.Elasticsearch
}

// newBuilder returns a new elasticsearch test builder
// with given name/namespace combination.
func newBuilder(name, namespace string) *esBuilder {
	return &esBuilder{
		es: &esv1.Elasticsearch{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		},
	}
}

// WithAnnotations adds the given annotations to the ES object.
func (e *esBuilder) WithAnnotations(annotations map[string]string) *esBuilder {
	e.es.ObjectMeta.Annotations = annotations
	return e
}

// WithGeneration adds the metadata.generation to the ES object.
func (e *esBuilder) WithGeneration(generation int64) *esBuilder {
	e.es.ObjectMeta.Generation = generation
	return e
}

// WithStatus adds the status subresource to the ES object.
func (e *esBuilder) WithStatus(status esv1.ElasticsearchStatus) *esBuilder {
	e.es.Status = status
	return e
}

// WithVersion adds the ES version to the ES objects specification.
func (e *esBuilder) WithVersion(version string) *esBuilder {
	e.es.Spec.Version = version
	return e
}

// Build builds the final ES object and returns a pointer.
func (e *esBuilder) Build() *esv1.Elasticsearch {
	return e.es
}

// BuildAndCopy builds the final ES object and returns a copy.
func (e *esBuilder) BuildAndCopy() esv1.Elasticsearch {
	return *e.es
}

var noInProgressOperations = esv1.InProgressOperations{
	DownscaleOperation: esv1.DownscaleOperation{
		LastUpdatedTime: metav1.Time{},
		Nodes:           nil,
	},
	UpgradeOperation: esv1.UpgradeOperation{
		LastUpdatedTime: metav1.Time{},
		Nodes:           nil,
	},
	UpscaleOperation: esv1.UpscaleOperation{
		LastUpdatedTime: metav1.Time{},
		Nodes:           nil,
	},
}

func TestReconcileElasticsearch_Reconcile(t *testing.T) {
	type k8sClientFields struct {
		objects []client.Object
	}
	type args struct {
		request reconcile.Request
	}
	tests := []struct {
		name            string
		k8sClientFields k8sClientFields
		args            args
		wantErr         bool
		expected        esv1.Elasticsearch
	}{
		{
			name: "unmanaged ES has no error, and no observedGeneration update",
			k8sClientFields: k8sClientFields{
				[]client.Object{
					newBuilder("testES", "test").
						WithGeneration(2).
						WithAnnotations(map[string]string{common.ManagedAnnotation: "false"}).
						WithStatus(esv1.ElasticsearchStatus{ObservedGeneration: 1}).Build(),
				},
			},
			args: args{
				request: reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      "testES",
						Namespace: "test",
					},
				},
			},
			wantErr: false,
			expected: newBuilder("testES", "test").
				WithGeneration(2).
				WithAnnotations(map[string]string{common.ManagedAnnotation: "false"}).
				WithStatus(esv1.ElasticsearchStatus{ObservedGeneration: 1}).BuildAndCopy(),
		},
		{
			name: "ES with too long name, fails initial reconcile, but has observedGeneration updated",
			k8sClientFields: k8sClientFields{
				[]client.Object{
					newBuilder("testESwithtoolongofanamereallylongname", "test").
						WithGeneration(2).
						WithAnnotations(map[string]string{hints.OrchestrationsHintsAnnotation: `{"no_transient_settings":false}`}).
						WithStatus(esv1.ElasticsearchStatus{ObservedGeneration: 1}).Build(),
				},
			},
			args: args{
				request: reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      "testESwithtoolongofanamereallylongname",
						Namespace: "test",
					},
				},
			},
			wantErr: false,
			expected: newBuilder("testESwithtoolongofanamereallylongname", "test").
				WithGeneration(2).
				WithAnnotations(map[string]string{hints.OrchestrationsHintsAnnotation: `{"no_transient_settings":false}`}).
				WithStatus(
					esv1.ElasticsearchStatus{
						ObservedGeneration:   2,
						Phase:                esv1.ElasticsearchResourceInvalid,
						Health:               esv1.ElasticsearchUnknownHealth,
						Conditions:           commonv1alpha1.Conditions{commonv1alpha1.Condition{Type: "ReconciliationComplete", Status: "True"}},
						InProgressOperations: noInProgressOperations,
					},
				).BuildAndCopy(),
		},
		{
			name: "Invalid ES with too long name, and updates observedGeneration",
			k8sClientFields: k8sClientFields{
				[]client.Object{
					newBuilder("testeswithtoolongofanamereallylongname", "test").
						WithGeneration(2).
						// we need two reconciliations here: first sets this annotation second updates status, this simulates that the first has happened
						WithAnnotations(map[string]string{hints.OrchestrationsHintsAnnotation: `{"no_transient_settings":false}`}).
						WithStatus(esv1.ElasticsearchStatus{ObservedGeneration: 1}).Build(),
				},
			},
			args: args{
				request: reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      "testeswithtoolongofanamereallylongname",
						Namespace: "test",
					},
				},
			},
			wantErr: false,
			expected: newBuilder("testeswithtoolongofanamereallylongname", "test").
				WithGeneration(2).
				WithAnnotations(map[string]string{hints.OrchestrationsHintsAnnotation: `{"no_transient_settings":false}`}).
				WithStatus(
					esv1.ElasticsearchStatus{
						ObservedGeneration:   2,
						Phase:                esv1.ElasticsearchResourceInvalid,
						Health:               esv1.ElasticsearchUnknownHealth,
						Conditions:           commonv1alpha1.Conditions{commonv1alpha1.Condition{Type: "ReconciliationComplete", Status: "True"}},
						InProgressOperations: noInProgressOperations,
					},
				).BuildAndCopy(),
		},
		{
			name: "Invalid ES version errors, and updates observedGeneration",
			k8sClientFields: k8sClientFields{
				[]client.Object{
					newBuilder("testES", "test").
						WithGeneration(2).
						WithVersion("invalid").
						WithAnnotations(map[string]string{hints.OrchestrationsHintsAnnotation: `{"no_transient_settings":false}`}).
						WithStatus(esv1.ElasticsearchStatus{ObservedGeneration: 1}).Build(),
				},
			},
			args: args{
				request: reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      "testES",
						Namespace: "test",
					},
				},
			},
			wantErr: false,
			expected: newBuilder("testES", "test").
				WithGeneration(2).
				WithVersion("invalid").
				WithAnnotations(map[string]string{hints.OrchestrationsHintsAnnotation: `{"no_transient_settings":false}`}).
				WithStatus(
					esv1.ElasticsearchStatus{
						ObservedGeneration:   2,
						Phase:                esv1.ElasticsearchResourceInvalid,
						Health:               esv1.ElasticsearchUnknownHealth,
						Conditions:           commonv1alpha1.Conditions{commonv1alpha1.Condition{Type: "ReconciliationComplete", Status: "True"}},
						InProgressOperations: noInProgressOperations,
					},
				).BuildAndCopy(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestReconciler(tt.k8sClientFields.objects...)
			if _, err := r.Reconcile(context.Background(), tt.args.request); (err != nil) != tt.wantErr {
				t.Errorf("ReconcileElasticsearch.Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			var actualES esv1.Elasticsearch
			if err := r.Client.Get(context.Background(), tt.args.request.NamespacedName, &actualES); err != nil {
				t.Error(err)
				return
			}
			comparison.AssertEqual(t, &actualES, &tt.expected)
		})
	}
}

func TestReconcileElasticsearch_LicensePasswordLength(t *testing.T) {
	testNS := "test"
	operatorNs := "elastic-system"
	es := newBuilder("test-es", testNS).
		WithVersion("8.0.0").
		Build()

	ctx := context.Background()

	generatorParams := commonpassword.GeneratorParams{
		LowerLetters: password.LowerLetters,
		UpperLetters: password.UpperLetters,
		Digits:       password.Digits,
		Symbols:      password.Symbols,
		Length:       32,
	}
	internalGenerator, err := password.NewGenerator(nil)
	require.NoError(t, err)

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-es",
			Namespace: testNS,
		},
	}

	// Create ES reconciler with basic license (no enterprise features)
	esReconciler := newTestReconciler(es)
	checker := license.NewLicenseChecker(esReconciler.Client, operatorNs)

	generator := commonpassword.NewRandomPasswordGenerator(internalGenerator, generatorParams, checker.EnterpriseFeaturesEnabled)
	params := operator.Parameters{
		PasswordGenerator: generator,
		OperatorNamespace: operatorNs,
	}
	esReconciler.Parameters = params
	esReconciler.licenseChecker = checker

	// Run ES reconciliation with basic license
	_, err = esReconciler.Reconcile(ctx, request)
	require.NoError(t, err)

	// Create internal users secret using user.ReconcileUsersAndRoles directly.:with
	// This simulates what the ES reconciler would call internally with 24-char passwords.
	_, err = user.ReconcileUsersAndRoles(ctx, esReconciler.Client, *es, watches.NewDynamicWatches(),
		record.NewFakeRecorder(100), &testPasswordHasher{}, esReconciler.Parameters.PasswordGenerator,
		metadata.Propagate(es, metadata.Metadata{Labels: es.GetIdentityLabels()}))
	require.NoError(t, err)

	// Verify actual secrets contain 24-character passwords (basic license)
	var internalUsersSecret corev1.Secret
	secretNSN := types.NamespacedName{
		Namespace: testNS,
		Name:      esv1.InternalUsersSecret("test-es"),
	}
	err = esReconciler.Client.Get(ctx, secretNSN, &internalUsersSecret)
	require.NoError(t, err, "Internal users secret should be created")

	require.NotEmpty(t, internalUsersSecret.Data, "Internal users secret should contain user passwords")
	for userKey, password := range internalUsersSecret.Data {
		require.Equal(t, 24, len(password), "Basic license password should be 24 characters for user %s", userKey)
	}

	// Ensure the operator namespace exists, and start an enterprise trial
	require.NoError(t, test.EnsureNamespace(esReconciler.Client, operatorNs))
	startTrial(t, esReconciler.Client)

	// Run ES controller reconcile with enterprise license active
	_, err = esReconciler.Reconcile(ctx, request)
	require.NoError(t, err)

	// Delete existing secrets to force regeneration
	err = esReconciler.Client.Delete(ctx, &internalUsersSecret)
	require.NoError(t, err)

	// Also delete the roles and file realm secret.
	var rolesSecret corev1.Secret
	rolesSecretNSN := types.NamespacedName{
		Namespace: testNS,
		Name:      esv1.RolesAndFileRealmSecret("test-es"),
	}
	err = esReconciler.Client.Get(ctx, rolesSecretNSN, &rolesSecret)
	if err == nil {
		require.NoError(t, esReconciler.Client.Delete(ctx, &rolesSecret))
	}

	// Recreate secrets with enterprise settings using user.ReconcileUsersAndRoles directly
	// This simulates what the ES reconciler would call internally with 32-char passwords
	_, err = user.ReconcileUsersAndRoles(ctx, esReconciler.Client, *es, watches.NewDynamicWatches(),
		record.NewFakeRecorder(100), &testPasswordHasher{}, esReconciler.Parameters.PasswordGenerator,
		metadata.Propagate(es, metadata.Metadata{Labels: es.GetIdentityLabels()}))
	require.NoError(t, err)

	// Verify actual secrets now contain 32-character passwords.
	err = esReconciler.Client.Get(ctx, secretNSN, &internalUsersSecret)
	require.NoError(t, err, "Internal users secret should be recreated with enterprise license")

	require.NotEmpty(t, internalUsersSecret.Data, "Internal users secret should contain user passwords")
	for userKey, password := range internalUsersSecret.Data {
		require.Equal(t, 32, len(password), "Enterprise license password should be 32 characters for user %s", userKey)
	}
}

// testPasswordHasher is a mock password hasher for testing
type testPasswordHasher struct{}

func (h *testPasswordHasher) GenerateHash(password []byte) ([]byte, error) {
	hash := make([]byte, len(password)+5)
	copy(hash[:5], "hash:")
	copy(hash[5:], password)
	return hash, nil
}

func (h *testPasswordHasher) ReuseOrGenerateHash(password []byte, existingHash []byte) ([]byte, error) {
	return h.GenerateHash(password)
}

func startTrial(t *testing.T, k8sClient client.Client) {
	t.Helper()
	// start a trial
	operatorNs := "elastic-system"
	trialState, err := license.NewTrialState()
	require.NoError(t, err)
	wrappedClient := k8sClient
	licenseNSN := types.NamespacedName{
		Namespace: operatorNs,
		Name:      "eck-trial",
	}
	// simulate user kicking off the trial activation
	require.NoError(t, license.CreateTrialLicense(context.Background(), wrappedClient, licenseNSN))
	// fetch user created license
	licenseSecret, lic, err := license.TrialLicense(wrappedClient, licenseNSN)
	require.NoError(t, err)
	// fill in and sign
	require.NoError(t, trialState.InitTrialLicense(context.Background(), &lic))
	status, err := license.ExpectedTrialStatus(operatorNs, licenseNSN, trialState)
	require.NoError(t, err)
	// persist status
	require.NoError(t, wrappedClient.Create(context.Background(), &status))
	// persist updated license
	require.NoError(t, license.UpdateEnterpriseLicense(context.Background(), wrappedClient, licenseSecret, lic))
}

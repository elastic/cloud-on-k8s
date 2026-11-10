// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"context"
	"errors"
	"maps"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	commonlabels "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/scheme"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func TestReconcileAutoOpsESCASecret(t *testing.T) {
	scheme.SetupScheme()

	policy := newAutoOpsAgentPolicy()
	es := newElasticsearch()

	publicSecretName := "es-1-es-http-certs-public"
	caSecretName := autoopsv1alpha1.CASecret(policy.GetName(), *es)

	tests := []struct {
		name           string
		publicSecret   *corev1.Secret
		esModifier     func(*esv1.Elasticsearch)
		wantSkip       bool
		wantCACertData []byte
	}{
		{
			name: "public secret has both ca.crt and tls.crt: uses ca.crt",
			publicSecret: &corev1.Secret{
				Name: publicSecretName, Namespace: "ns-1",
				Data: map[string][]byte{
					certificates.CAFileName:   []byte("custom-ca-cert"),
					certificates.CertFileName: []byte("server-tls-cert"),
				},
			},
			wantCACertData: []byte("custom-ca-cert"),
		},
		{
			name: "public secret has only tls.crt (self-signed): falls back to tls.crt",
			publicSecret: &corev1.Secret{
				Name: publicSecretName, Namespace: "ns-1",
				Data: map[string][]byte{
					certificates.CertFileName: []byte("self-signed-cert-with-ca-chain"),
				},
			},
			wantCACertData: []byte("self-signed-cert-with-ca-chain"),
		},
		{
			name: "public secret has ca.crt but not tls.crt: uses ca.crt",
			publicSecret: &corev1.Secret{
				Name: publicSecretName, Namespace: "ns-1",
				Data: map[string][]byte{
					certificates.CAFileName: []byte("ca-only-cert"),
				},
			},
			wantCACertData: []byte("ca-only-cert"),
		},
		{
			name: "public secret has neither ca.crt nor tls.crt: skipped",
			publicSecret: &corev1.Secret{
				Name: publicSecretName, Namespace: "ns-1",
				Data: map[string][]byte{},
			},
			wantSkip: true,
		},
		{
			name:         "public secret not found: skipped",
			publicSecret: nil,
			wantSkip:     true,
		},
		{
			name: "ES not ready: skipped",
			publicSecret: &corev1.Secret{
				Name: publicSecretName, Namespace: "ns-1",
				Data: map[string][]byte{
					certificates.CAFileName: []byte("ca-cert"),
				},
			},
			esModifier: func(e *esv1.Elasticsearch) {
				e.Status.Phase = esv1.ElasticsearchApplyingChangesPhase
			},
			wantSkip: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := []client.Object{}
			if tt.publicSecret != nil {
				objects = append(objects, tt.publicSecret)
			}
			k8sClient := k8s.NewFakeClient(objects...)

			r := &AgentPolicyReconciler{
				Client:         k8sClient,
				dynamicWatches: watches.NewDynamicWatches(),
			}

			testES := *es
			if tt.esModifier != nil {
				tt.esModifier(&testES)
			}
			err := r.reconcileAutoOpsESCASecret(t.Context(), policy, testES)
			require.NoError(t, err)

			var caSecret corev1.Secret
			getErr := k8sClient.Get(t.Context(), types.NamespacedName{Name: caSecretName, Namespace: "ns-1"}, &caSecret)

			if tt.wantSkip {
				assert.True(t, apierrors.IsNotFound(getErr), "expected AutoOps CA secret to not be created")
				return
			}

			require.NoError(t, getErr, "expected AutoOps CA secret to be created")
			assert.Equal(t, tt.wantCACertData, caSecret.Data[certificates.CAFileName],
				"AutoOps CA secret should contain the expected CA certificate")
		})
	}
}

func TestBuildAutoOpsESCASecretDoesNotMutatePolicyLabels(t *testing.T) {
	policy := newAutoOpsAgentPolicy(func(p *autoopsv1alpha1.AutoOpsAgentPolicy) {
		p.Labels = map[string]string{"user-label": "value"}
	})
	es := newElasticsearch()

	secret := buildAutoOpsESCASecret(policy, *es, "ca-secret", []byte("ca-cert"))

	assert.Equal(t, commonv1.RestrictWatchedResourcesLabelValue, secret.Labels[commonv1.RestrictWatchedResourcesLabelName])
	assert.Equal(t, "value", secret.Labels["user-label"])
	// the policy's own labels must not be polluted with the secret's labels
	assert.Equal(t, map[string]string{"user-label": "value"}, policy.Labels)
}

func TestReconcileAutoOpsESClientCertSecretDoesNotMutatePolicy(t *testing.T) {
	scheme.SetupScheme()

	policy := newAutoOpsAgentPolicy(func(p *autoopsv1alpha1.AutoOpsAgentPolicy) {
		p.Labels = map[string]string{"user-label": "value"}
		p.Annotations = map[string]string{"user-annotation": "value"}
	})
	es := newElasticsearch()

	r := &AgentPolicyReconciler{
		Client:         k8s.NewFakeClient(),
		dynamicWatches: watches.NewDynamicWatches(),
	}

	secret, results := r.reconcileAutoOpsESClientCertSecret(t.Context(), policy, *es)
	_, err := results.Aggregate()
	require.NoError(t, err)
	require.NotNil(t, secret)

	assert.Equal(t, "true", secret.Labels[commonlabels.ClientCertificateLabelName])
	assert.Equal(t, "value", secret.Labels["user-label"])
	// the policy's own labels and annotations must not be polluted with the secret's
	assert.Equal(t, map[string]string{"user-label": "value"}, policy.Labels)
	assert.Equal(t, map[string]string{"user-annotation": "value"}, policy.Annotations)
}

func TestReconcileAutoOpsESClientCertSecretTrustSourceMetadata(t *testing.T) {
	scheme.SetupScheme()

	es := newElasticsearch(func(es *esv1.Elasticsearch) {
		es.Namespace = "es-ns"
	})
	tests := []struct {
		name            string
		policyLabels    map[string]string
		policyAnnots    map[string]string
		wantLabels      map[string]string
		wantAnnotations map[string]string
		wantNoAnnots    []string
	}{
		{
			name: "user labels and annotations are not copied into the ES namespace",
			policyLabels: map[string]string{
				"user-label": "value",
			},
			policyAnnots: map[string]string{
				corev1.LastAppliedConfigAnnotation: "{}",
				"user-annotation":                  "value",
			},
			wantNoAnnots: []string{corev1.LastAppliedConfigAnnotation, "user-annotation"},
		},
		{
			name: "only the metadata explicitly propagated is copied",
			policyLabels: map[string]string{
				"propagated-label": "value",
				"user-label":       "value",
			},
			policyAnnots: map[string]string{
				annotation.PropagateLabelsAnnotation:      "propagated-label",
				annotation.PropagateAnnotationsAnnotation: "propagated-annotation",
				"propagated-annotation":                   "value",
				"user-annotation":                         "value",
			},
			wantLabels:      map[string]string{"propagated-label": "value"},
			wantAnnotations: map[string]string{"propagated-annotation": "value"},
			wantNoAnnots:    []string{"user-annotation"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := newAutoOpsAgentPolicy(func(p *autoopsv1alpha1.AutoOpsAgentPolicy) {
				p.Labels = tt.policyLabels
				p.Annotations = tt.policyAnnots
			})
			c := k8s.NewFakeClient()
			r := &AgentPolicyReconciler{Client: c, dynamicWatches: watches.NewDynamicWatches()}

			clientCert, results := r.reconcileAutoOpsESClientCertSecret(t.Context(), policy, *es)
			_, err := results.Aggregate()
			require.NoError(t, err)
			require.NotNil(t, clientCert)

			var trustSources corev1.SecretList
			require.NoError(t, c.List(t.Context(), &trustSources,
				client.InNamespace(es.Namespace),
				client.MatchingLabels{commonv1.TypeLabelName: certificates.ClientCertTrustSourceType},
			))
			require.Len(t, trustSources.Items, 1)
			got := trustSources.Items[0]

			wantLabels := resourceLabelsFor(policy, *es)
			wantLabels[commonv1.TypeLabelName] = certificates.ClientCertTrustSourceType
			wantLabels[policySecretTypeLabelKey] = clientCertSecretType
			// added by the secret reconciliation so that the secret is watched
			wantLabels[commonv1.RestrictWatchedResourcesLabelName] = commonv1.RestrictWatchedResourcesLabelValue
			maps.Copy(wantLabels, tt.wantLabels)
			assert.Equal(t, wantLabels, got.Labels)
			for k, v := range tt.wantAnnotations {
				assert.Equal(t, v, got.Annotations[k])
			}
			for _, k := range tt.wantNoAnnots {
				assert.NotContains(t, got.Annotations, k)
			}
		})
	}
}

func TestAutoOpsESClientCertTrustSource(t *testing.T) {
	scheme.SetupScheme()

	policy := newAutoOpsAgentPolicy()
	// The ES runs in another namespace than the policy: its trust bundle only includes trust sources published
	// into its own namespace.
	es := newElasticsearch(func(es *esv1.Elasticsearch) {
		es.Namespace = "es-ns"
	})
	clientCertNSN := types.NamespacedName{Namespace: policy.Namespace, Name: autoopsv1alpha1.ClientCertSecret(policy.Name, *es)}
	// legacyClientCert is the client cert secret as created by older operator versions, with the soft-owner labels,
	// which are not used any more.
	legacyClientCert := &corev1.Secret{
		Namespace: clientCertNSN.Namespace,
		Name:      clientCertNSN.Name,
		Labels: map[string]string{
			reconciler.SoftOwnerNameLabel:      es.Name,
			reconciler.SoftOwnerNamespaceLabel: es.Namespace,
			reconciler.SoftOwnerKindLabel:      esv1.Kind,
		},
		Data: map[string][]byte{certificates.CertFileName: []byte("legacy-cert")},
	}

	// otherPolicy publishes its own client certificate for the same ES: it must be left alone by every cleanup of
	// the policy.
	otherPolicy := newAutoOpsAgentPolicy()
	otherPolicy.Name = "other-policy"
	otherClientCertNSN := types.NamespacedName{Namespace: otherPolicy.Namespace, Name: autoopsv1alpha1.ClientCertSecret(otherPolicy.Name, *es)}
	policyRequest := reconcile.Request{NamespacedName: k8s.ExtractNamespacedName(&policy)}
	failTrustSourceCreation := &interceptor.Funcs{
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			if obj.GetLabels()[commonv1.TypeLabelName] == certificates.ClientCertTrustSourceType {
				return errors.New("simulated trust source create failure")
			}
			return c.Create(ctx, obj, opts...)
		},
	}

	failTrustSourceDeletion := &interceptor.Funcs{
		Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
			if obj.GetLabels()[commonv1.TypeLabelName] == certificates.ClientCertTrustSourceType {
				return errors.New("simulated trust source delete failure")
			}
			return c.Delete(ctx, obj, opts...)
		},
	}
	// sameNamePolicy has the name of the policy, in the ES namespace, where the trust sources of the policy are
	// published.
	sameNamePolicy := newAutoOpsAgentPolicy()
	sameNamePolicy.Namespace = es.Namespace

	for _, tt := range []struct {
		name string
		objs []client.Object
		// interceptorFuncs makes the client fail.
		interceptorFuncs *interceptor.Funcs
		// cleanup runs after the client certificates have been reconciled.
		cleanup func(t *testing.T, r *AgentPolicyReconciler)
		// wantReconcileErr is the error expected from the reconciliation of the policy client certificate.
		wantReconcileErr string
		wantTrustSources []types.NamespacedName
		// wantRepublish are the requests enqueued when the trust source is deleted: the policy is reconciled, to
		// publish it again, as long as the trust source is expected.
		wantRepublish []reconcile.Request
	}{
		{
			name:             "publishes the client certificate into the ES namespace",
			wantTrustSources: []types.NamespacedName{clientCertNSN, otherClientCertNSN},
			wantRepublish:    []reconcile.Request{policyRequest},
		},
		{
			name:             "publishes the client certificate of a secret created by an older operator version",
			objs:             []client.Object{legacyClientCert},
			wantTrustSources: []types.NamespacedName{clientCertNSN, otherClientCertNSN},
			wantRepublish:    []reconcile.Request{policyRequest},
		},
		{
			// The ES must not trust a client certificate the policy is not given.
			name:             "publishes nothing and returns the error when the trust source cannot be created",
			interceptorFuncs: failTrustSourceCreation,
			wantReconcileErr: "simulated trust source create failure",
		},
		{
			name: "deletes the trust source when client authentication is no longer required",
			cleanup: func(t *testing.T, r *AgentPolicyReconciler) {
				t.Helper()
				require.NoError(t, r.deleteAutoOpsESClientCertSecret(t.Context(), policy, *es))
			},
			wantTrustSources: []types.NamespacedName{otherClientCertNSN},
		},
		{
			name: "deletes the trust source when the ES is no longer selected",
			cleanup: func(t *testing.T, r *AgentPolicyReconciler) {
				t.Helper()
				require.NoError(t, r.cleanupOrphanedSecrets(t.Context(), logr.Discard(), policy, client.MatchingLabels{PolicyNameLabelKey: policy.Name}, sets.New[types.NamespacedName]()))
			},
			wantTrustSources: []types.NamespacedName{otherClientCertNSN},
		},
		{
			name: "keeps the trust source when the ES is still selected",
			cleanup: func(t *testing.T, r *AgentPolicyReconciler) {
				t.Helper()
				require.NoError(t, r.cleanupOrphanedSecrets(t.Context(), logr.Discard(), policy, client.MatchingLabels{PolicyNameLabelKey: policy.Name}, sets.New(k8s.ExtractNamespacedName(es))))
			},
			wantTrustSources: []types.NamespacedName{clientCertNSN, otherClientCertNSN},
			wantRepublish:    []reconcile.Request{policyRequest},
		},
		{
			name: "deletes the trust source when the policy is deleted",
			cleanup: func(t *testing.T, r *AgentPolicyReconciler) {
				t.Helper()
				require.NoError(t, r.onDelete(t.Context(), k8s.ExtractNamespacedName(&policy)))
			},
			wantTrustSources: []types.NamespacedName{otherClientCertNSN},
		},
		{
			name: "keeps the trust source when a policy with the same name in the ES namespace does not select the ES",
			cleanup: func(t *testing.T, r *AgentPolicyReconciler) {
				t.Helper()
				require.NoError(t, r.cleanupOrphanedSecrets(t.Context(), logr.Discard(), sameNamePolicy, client.MatchingLabels{PolicyNameLabelKey: sameNamePolicy.Name}, sets.New[types.NamespacedName]()))
			},
			wantTrustSources: []types.NamespacedName{clientCertNSN, otherClientCertNSN},
			wantRepublish:    []reconcile.Request{policyRequest},
		},
		{
			name:             "returns the error when the trust source cannot be deleted on policy deletion",
			interceptorFuncs: failTrustSourceDeletion,
			cleanup: func(t *testing.T, r *AgentPolicyReconciler) {
				t.Helper()
				require.ErrorContains(t, r.onDelete(t.Context(), k8s.ExtractNamespacedName(&policy)), "simulated trust source delete failure")
			},
			wantTrustSources: []types.NamespacedName{clientCertNSN, otherClientCertNSN},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			builder := k8s.NewFakeClientBuilder(append([]client.Object{es}, tt.objs...)...)
			if tt.interceptorFuncs != nil {
				builder = builder.WithInterceptorFuncs(*tt.interceptorFuncs)
			}
			r := &AgentPolicyReconciler{
				Client:           builder.Build(),
				esClientProvider: newFakeESClientProvider().Provider,
				params:           operator.Parameters{Dialer: &fakeDialer{}},
				dynamicWatches:   watches.NewDynamicWatches(),
			}
			clientCert, results := r.reconcileAutoOpsESClientCertSecret(t.Context(), policy, *es)
			_, err := results.Aggregate()
			gotReconcileErr := ""
			if err != nil {
				gotReconcileErr = err.Error()
			}
			require.Equal(t, tt.wantReconcileErr == "", err == nil)
			require.Contains(t, gotReconcileErr, tt.wantReconcileErr)
			require.Equal(t, tt.wantReconcileErr == "", clientCert != nil, "the client certificate must only be used when the ES trusts it")

			// the other policy is reconciled too: whether it succeeds is checked through its trust source
			_, _ = r.reconcileAutoOpsESClientCertSecret(t.Context(), otherPolicy, *es)

			if tt.cleanup != nil {
				tt.cleanup(t, r)
			}

			trustSources, err := certificates.ListClientCertTrustSources(t.Context(), r.Client, nil)
			require.NoError(t, err)
			var gotTrustSources []types.NamespacedName //nolint:prealloc
			for _, ts := range trustSources {
				require.Equal(t, es.Namespace, ts.Namespace, "trust sources must be published into the ES namespace")
				origin, _ := certificates.ClientCertTrustSourceOrigin(ts)
				gotTrustSources = append(gotTrustSources, origin)
			}
			require.ElementsMatch(t, tt.wantTrustSources, gotTrustSources)

			// The deletion of the trust source, which the ES owns, makes the policy publish it again.
			require.Equal(t, tt.wantRepublish, requestsOnSecretDeletion(t, r.dynamicWatches.Secrets, certificates.ClientCertTrustSourceKey(es.Namespace, clientCertNSN)))

			// The client cert secret does not carry server soft-owner labels.
			var secrets corev1.SecretList
			require.NoError(t, r.Client.List(t.Context(), &secrets, client.InNamespace(policy.Namespace)))
			for _, s := range secrets.Items {
				require.NotContains(t, s.Labels, reconciler.SoftOwnerKindLabel, "secret %s", s.Name)
			}
		})
	}
}

// requestsOnSecretDeletion returns the reconcile requests the given watches enqueue when the Secret key is deleted.
func requestsOnSecretDeletion(t *testing.T, secretWatches *watches.DynamicEnqueueRequest[*corev1.Secret], key types.NamespacedName) []reconcile.Request {
	t.Helper()
	q := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
	defer q.ShutDown()
	secretWatches.Delete(t.Context(), event.TypedDeleteEvent[*corev1.Secret]{Object: &corev1.Secret{ObjectMeta: k8s.ToObjectMeta(key)}}, q)

	var requests []reconcile.Request
	for q.Len() > 0 {
		req, _ := q.Get()
		q.Done(req)
		requests = append(requests, req)
	}
	return requests
}

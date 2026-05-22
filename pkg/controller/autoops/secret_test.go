// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
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
				ObjectMeta: metav1.ObjectMeta{Name: publicSecretName, Namespace: "ns-1"},
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
				ObjectMeta: metav1.ObjectMeta{Name: publicSecretName, Namespace: "ns-1"},
				Data: map[string][]byte{
					certificates.CertFileName: []byte("self-signed-cert-with-ca-chain"),
				},
			},
			wantCACertData: []byte("self-signed-cert-with-ca-chain"),
		},
		{
			name: "public secret has ca.crt but not tls.crt: uses ca.crt",
			publicSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: publicSecretName, Namespace: "ns-1"},
				Data: map[string][]byte{
					certificates.CAFileName: []byte("ca-only-cert"),
				},
			},
			wantCACertData: []byte("ca-only-cert"),
		},
		{
			name: "public secret has neither ca.crt nor tls.crt: skipped",
			publicSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: publicSecretName, Namespace: "ns-1"},
				Data:       map[string][]byte{},
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
				ObjectMeta: metav1.ObjectMeta{Name: publicSecretName, Namespace: "ns-1"},
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

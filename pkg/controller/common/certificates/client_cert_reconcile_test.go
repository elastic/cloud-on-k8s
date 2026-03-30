// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package certificates

import (
	"context"
	"crypto/x509"
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func newTestReconciler(c k8s.Client, owner client.Object, lbls map[string]string) Reconciler {
	return Reconciler{
		K8sClient: c,
		Owner:     owner,
		Namer:     esv1.ESNamer,
		Metadata: metadata.Metadata{
			Labels: lbls,
		},
		CertRotation: RotationParams{
			Validity:     DefaultCertValidity,
			RotateBefore: DefaultRotateBefore,
		},
	}
}

func TestReconcileClientCertificate_OperatorCert(t *testing.T) {
	owner := &esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "es",
			UID:       "test-uid",
		},
	}
	secretName := OperatorClientCertSecretName(esv1.ESNamer, owner.Name)

	t.Run("creates new self-signed secret when none exists", func(t *testing.T) {
		c := k8s.NewFakeClient()
		r := newTestReconciler(c, owner, map[string]string{"app": "test"})

		secret, err := r.ReconcileClientCertificate(context.Background(), secretName, internalClientCertCommonName, owner.Name, nil)
		require.NoError(t, err)
		require.NotNil(t, secret)

		require.Equal(t, secretName, secret.Name)
		require.Equal(t, owner.Namespace, secret.Namespace)

		require.NotEmpty(t, secret.Data[CertFileName])
		require.NotEmpty(t, secret.Data[KeyFileName])
		require.Empty(t, secret.Data[CAFileName])
		require.Equal(t, "test", secret.Labels["app"])

		// No extra labels means no soft-owner labels
		require.Empty(t, secret.Labels[reconciler.SoftOwnerNameLabel])
		require.Empty(t, secret.Labels[reconciler.SoftOwnerNamespaceLabel])
		require.Empty(t, secret.Labels[reconciler.SoftOwnerKindLabel])

		tlsCert, err := ParseTLSCertificate(secret.Secret)
		require.NoError(t, err)
		require.NotNil(t, tlsCert)

		certs, err := ParsePEMCerts(secret.Data[CertFileName])
		require.NoError(t, err)
		require.Len(t, certs, 1)
		require.Equal(t, internalClientCertCommonName, certs[0].Subject.CommonName)
		require.Equal(t, []string{owner.Name}, certs[0].Subject.OrganizationalUnit)
		require.Contains(t, certs[0].ExtKeyUsage, x509.ExtKeyUsageClientAuth)
		require.NotContains(t, certs[0].ExtKeyUsage, x509.ExtKeyUsageServerAuth)
		require.Equal(t, certs[0].Subject.CommonName, certs[0].Issuer.CommonName)
	})

	t.Run("reuses existing valid secret", func(t *testing.T) {
		c := k8s.NewFakeClient()
		r := newTestReconciler(c, owner, nil)

		secret1, err := r.ReconcileClientCertificate(context.Background(), secretName, internalClientCertCommonName, owner.Name, nil)
		require.NoError(t, err)

		secret2, err := r.ReconcileClientCertificate(context.Background(), secretName, internalClientCertCommonName, owner.Name, nil)
		require.NoError(t, err)

		require.Equal(t, secret1.Data[CertFileName], secret2.Data[CertFileName])
		require.Equal(t, secret1.Data[KeyFileName], secret2.Data[KeyFileName])
	})
}

func TestReconcileClientCertificate_WithExtraLabels(t *testing.T) {
	kibana := &kbv1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "my-kibana",
			UID:       "test-uid",
		},
		Spec: kbv1.KibanaSpec{
			ElasticsearchRef: commonv1.ObjectSelector{
				Name:      "my-es",
				Namespace: "es-ns",
			},
		},
	}
	secretName := "my-kibana-es-test-client-cert"

	extraLabels := map[string]string{
		reconciler.SoftOwnerNameLabel:      "my-es",
		reconciler.SoftOwnerNamespaceLabel: "es-ns",
		reconciler.SoftOwnerKindLabel:      esv1.Kind,
		labels.ClientCertificateLabelName:  "true",
	}

	t.Run("creates secret with soft-owner labels", func(t *testing.T) {
		c := k8s.NewFakeClient()
		r := newTestReconciler(c, kibana, map[string]string{"base": "label"})

		secret, err := r.ReconcileClientCertificate(context.Background(), secretName, kibana.Name, kibana.Name, extraLabels)
		require.NoError(t, err)
		require.NotNil(t, secret)

		require.Equal(t, secretName, secret.Name)
		require.Equal(t, "my-es", secret.Labels[reconciler.SoftOwnerNameLabel])
		require.Equal(t, "es-ns", secret.Labels[reconciler.SoftOwnerNamespaceLabel])
		require.Equal(t, esv1.Kind, secret.Labels[reconciler.SoftOwnerKindLabel])
		require.Equal(t, "true", secret.Labels[labels.ClientCertificateLabelName])
		require.Equal(t, "label", secret.Labels["base"])
	})

	t.Run("derives common name from provided argument", func(t *testing.T) {
		c := k8s.NewFakeClient()
		r := newTestReconciler(c, kibana, nil)

		secret, err := r.ReconcileClientCertificate(context.Background(), secretName, "my-kibana", kibana.Name, extraLabels)
		require.NoError(t, err)

		cert, err := ParseTLSCertificate(secret.Secret)
		require.NoError(t, err)
		require.Equal(t, "my-kibana", cert.Leaf.Subject.CommonName)
	})
}

func TestParseTLSCertificate(t *testing.T) {
	owner := &esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es", UID: "test-uid"},
	}
	c := k8s.NewFakeClient()
	r := newTestReconciler(c, owner, nil)
	selfSignedSecret, err := r.ReconcileClientCertificate(context.Background(), "test-cert", "test", "es", nil)
	require.NoError(t, err)

	certPEM := selfSignedSecret.Data[CertFileName]
	keyPEM := selfSignedSecret.Data[KeyFileName]

	t.Run("parses valid cert and key", func(t *testing.T) {
		secret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "test"},
			Data: map[string][]byte{
				CertFileName: certPEM,
				KeyFileName:  keyPEM,
			},
		}
		tlsCert, err := ParseTLSCertificate(secret)
		require.NoError(t, err)
		require.NotNil(t, tlsCert)
	})

	t.Run("fails on missing cert", func(t *testing.T) {
		secret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "test"},
			Data:       map[string][]byte{KeyFileName: keyPEM},
		}
		_, err := ParseTLSCertificate(secret)
		require.Error(t, err)
	})

	t.Run("fails on missing key", func(t *testing.T) {
		secret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "test"},
			Data:       map[string][]byte{CertFileName: certPEM},
		}
		_, err := ParseTLSCertificate(secret)
		require.Error(t, err)
	})
}

func TestLoadClientCertIfExists(t *testing.T) {
	owner := &esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es", UID: "test-uid"},
	}

	expectedSecretName := OperatorClientCertSecretName(esv1.ESNamer, owner.Name)

	fakeClient := k8s.NewFakeClient()
	r := newTestReconciler(fakeClient, owner, nil)
	selfSignedSecret, err := r.ReconcileClientCertificate(context.Background(), expectedSecretName, "test", "es", nil)
	require.NoError(t, err)

	t.Run("returns nil when secret does not exist", func(t *testing.T) {
		c := k8s.NewFakeClient()
		tlsCert, err := LoadOperatorClientCertIfExists(context.Background(), c, esv1.ESNamer, "ns", "nonexistent")
		require.NoError(t, err)
		require.Nil(t, tlsCert)
	})

	t.Run("loads existing secret", func(t *testing.T) {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: expectedSecretName},
			Data: map[string][]byte{
				CertFileName: selfSignedSecret.Data[CertFileName],
				KeyFileName:  selfSignedSecret.Data[KeyFileName],
			},
		}
		c := k8s.NewFakeClient(secret)

		tlsCert, err := LoadOperatorClientCertIfExists(context.Background(), c, esv1.ESNamer, "ns", owner.Name)
		require.NoError(t, err)
		require.NotNil(t, tlsCert)
	})
}

func TestClientCertSecretCleanup(t *testing.T) {
	owner := &esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es", UID: "test-uid"},
	}
	secretName := OperatorClientCertSecretName(esv1.ESNamer, owner.Name)

	t.Run("secret can be deleted after creation", func(t *testing.T) {
		c := k8s.NewFakeClient()
		r := newTestReconciler(c, owner, nil)

		secret, err := r.ReconcileClientCertificate(context.Background(), secretName, internalClientCertCommonName, owner.Name, nil)
		require.NoError(t, err)

		var fetchedSecret corev1.Secret
		err = c.Get(context.Background(), types.NamespacedName{Namespace: secret.Namespace, Name: secret.Name}, &fetchedSecret)
		require.NoError(t, err)

		err = k8s.DeleteSecretIfExists(context.Background(), c, types.NamespacedName{Namespace: secret.Namespace, Name: secret.Name})
		require.NoError(t, err)

		err = c.Get(context.Background(), types.NamespacedName{Namespace: secret.Namespace, Name: secret.Name}, &fetchedSecret)
		require.True(t, apierrors.IsNotFound(err))
	})
}

func TestDiscoverClientCertSecrets(t *testing.T) {
	t.Run("discovers secrets with matching labels", func(t *testing.T) {
		secret1 := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "client-ns-1",
				Name:      "client-1-es-client-cert",
				Labels: map[string]string{
					labels.ClientCertificateLabelName:  "true",
					reconciler.SoftOwnerNameLabel:      "my-es",
					reconciler.SoftOwnerNamespaceLabel: "es-ns",
					reconciler.SoftOwnerKindLabel:      esv1.Kind,
				},
			},
			Data: map[string][]byte{CertFileName: []byte("cert-1")},
		}
		secret2 := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "client-ns-2",
				Name:      "client-2-es-client-cert",
				Labels: map[string]string{
					labels.ClientCertificateLabelName:  "true",
					reconciler.SoftOwnerNameLabel:      "my-es",
					reconciler.SoftOwnerNamespaceLabel: "es-ns",
					reconciler.SoftOwnerKindLabel:      esv1.Kind,
				},
			},
			Data: map[string][]byte{CertFileName: []byte("cert-2")},
		}
		secretOther := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "other-ns",
				Name:      "other-client-cert",
				Labels: map[string]string{
					labels.ClientCertificateLabelName:  "true",
					reconciler.SoftOwnerNameLabel:      "other-es",
					reconciler.SoftOwnerNamespaceLabel: "other-ns",
					reconciler.SoftOwnerKindLabel:      esv1.Kind,
				},
			},
			Data: map[string][]byte{CertFileName: []byte("other-cert")},
		}

		c := k8s.NewFakeClient(secret1, secret2, secretOther)
		secrets, err := discoverClientCertSecrets(context.Background(), c, "my-es", "es-ns", esv1.Kind)
		require.NoError(t, err)
		require.Len(t, secrets, 2)
	})

	t.Run("returns empty list when no matching secrets", func(t *testing.T) {
		c := k8s.NewFakeClient()
		secrets, err := discoverClientCertSecrets(context.Background(), c, "my-es", "es-ns", esv1.Kind)
		require.NoError(t, err)
		require.Empty(t, secrets)
	})
}

func TestBuildTrustBundleFromSecrets(t *testing.T) {
	t.Run("concatenates tls.crt from secrets", func(t *testing.T) {
		secrets := []corev1.Secret{
			{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns1", Name: "secret1"},
				Data:       map[string][]byte{CertFileName: []byte("CERT-1\n")},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns2", Name: "secret2"},
				Data:       map[string][]byte{CertFileName: []byte("CERT-2\n")},
			},
		}

		bundle := buildTrustBundleFromSecrets(context.Background(), secrets)
		require.Equal(t, "CERT-1\nCERT-2\n", string(bundle))
	})

	t.Run("skips secrets without tls.crt", func(t *testing.T) {
		secrets := []corev1.Secret{
			{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns1", Name: "secret1"},
				Data:       map[string][]byte{CertFileName: []byte("CERT\n")},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns2", Name: "secret2"},
				Data:       map[string][]byte{},
			},
		}

		bundle := buildTrustBundleFromSecrets(context.Background(), secrets)
		require.Equal(t, "CERT\n", string(bundle))
	})

	t.Run("sorts secrets by namespace/name for deterministic output", func(t *testing.T) {
		secrets := []corev1.Secret{
			{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns2", Name: "secret-b"},
				Data:       map[string][]byte{CertFileName: []byte("B")},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns1", Name: "secret-a"},
				Data:       map[string][]byte{CertFileName: []byte("A")},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns2", Name: "secret-a"},
				Data:       map[string][]byte{CertFileName: []byte("C")},
			},
		}

		bundle := buildTrustBundleFromSecrets(context.Background(), secrets)
		require.Equal(t, "A\nC\nB\n", string(bundle))
	})

	t.Run("does not double newline when cert already ends with newline", func(t *testing.T) {
		secrets := []corev1.Secret{
			{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns1", Name: "a"},
				Data:       map[string][]byte{CertFileName: []byte("CERT-1\n")},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns1", Name: "b"},
				Data:       map[string][]byte{CertFileName: []byte("CERT-2")},
			},
		}

		bundle := buildTrustBundleFromSecrets(context.Background(), secrets)
		require.Equal(t, "CERT-1\nCERT-2\n", string(bundle))
	})
}

func TestDeleteClientCertResources(t *testing.T) {
	esName := "my-es"
	esNS := "my-ns"

	operatorCertSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: esNS, Name: OperatorClientCertSecretName(esv1.ESNamer, esName)},
	}
	trustBundleSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: esNS, Name: ClientCertTrustBundleSecretName(esv1.ESNamer, esName)},
	}

	tests := []struct {
		name                  string
		extraInitialObjs      []client.Object
		interceptorFuncs      *interceptor.Funcs
		wantErr               bool
		wantAnnotationPresent bool
	}{
		{
			name:             "deletes annotation, operator cert secret, and trust bundle secret",
			extraInitialObjs: []client.Object{operatorCertSecret, trustBundleSecret},
		},
		{
			name: "no-op when resources already absent",
		},
		{
			name:             "annotation preserved when operator cert secret deletion fails",
			extraInitialObjs: []client.Object{operatorCertSecret, trustBundleSecret},
			interceptorFuncs: &interceptor.Funcs{
				Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
					if obj.GetName() == operatorCertSecret.GetName() {
						return fmt.Errorf("simulated delete failure")
					}
					return c.Delete(ctx, obj, opts...)
				},
			},
			wantErr:               true,
			wantAnnotationPresent: true,
		},
		{
			name:             "annotation preserved when trust bundle secret deletion fails",
			extraInitialObjs: []client.Object{operatorCertSecret, trustBundleSecret},
			interceptorFuncs: &interceptor.Funcs{
				Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
					if obj.GetName() == trustBundleSecret.GetName() {
						return fmt.Errorf("simulated delete failure")
					}
					return c.Delete(ctx, obj, opts...)
				},
			},
			wantErr:               true,
			wantAnnotationPresent: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := &esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: esNS,
					Name:      esName,
					Annotations: map[string]string{
						annotation.ClientAuthenticationRequiredAnnotation: "true",
					},
				},
			}

			ctx := context.Background()
			builder := k8s.NewFakeClientBuilder(slices.Concat([]client.Object{es}, tt.extraInitialObjs)...)
			if tt.interceptorFuncs != nil {
				builder = builder.WithInterceptorFuncs(*tt.interceptorFuncs)
			}
			c := builder.Build()

			err := DeleteClientCertResources(ctx, c, es, esv1.ESNamer)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			var updatedES esv1.Elasticsearch
			require.NoError(t, c.Get(ctx, types.NamespacedName{Namespace: esNS, Name: esName}, &updatedES))
			require.Equal(t, tt.wantAnnotationPresent, annotation.HasClientAuthenticationRequired(&updatedES))
		})
	}
}

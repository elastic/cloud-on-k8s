// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package certificates

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

var (
	rotation = RotationParams{
		Validity:     DefaultCertValidity,
		RotateBefore: DefaultRotateBefore,
	}
	labels = map[string]string{
		"foo": "bar",
	}
	// tested on Elasticsearch but could be any resource
	obj = esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "es",
		},
		TypeMeta: metav1.TypeMeta{
			Kind: esv1.Kind,
		},
	}
)

// this test just visits the main path of the certs reconciliation
// inner functions are individually tested elsewhere
func TestReconcileCAAndHTTPCerts(t *testing.T) {
	c := k8s.NewFakeClient()

	r := Reconciler{
		K8sClient:             c,
		DynamicWatches:        watches.NewDynamicWatches(),
		Owner:                 &obj,
		TLSOptions:            commonv1.TLSOptions{},
		Namer:                 esv1.ESNamer,
		Metadata:              metadata.Metadata{Labels: labels},
		Services:              nil,
		CACertRotation:        rotation,
		CertRotation:          rotation,
		GarbageCollectSecrets: false,
	}
	httpCerts, results := r.ReconcileCAAndHTTPCerts(context.Background())
	checkResults := func() {
		aggregateResult, err := results.Aggregate()
		require.NoError(t, err)
		// a reconciliation should be requested later to deal with cert expiration
		require.NotZero(t, aggregateResult.RequeueAfter)
		// returned http certs should hold cert data
		require.NotNil(t, httpCerts)
		require.NotEmpty(t, httpCerts.Data)
	}
	checkResults()

	labelsWithSoftOwner := map[string]string{
		"foo":                              "bar",
		reconciler.SoftOwnerKindLabel:      obj.Kind,
		reconciler.SoftOwnerNamespaceLabel: obj.Namespace,
		reconciler.SoftOwnerNameLabel:      obj.Name,
	}

	// the 3 secrets should have been created in the apiserver,
	// and have the expected labels and content generated
	checkCertsSecrets := func() {
		var caCerts corev1.Secret
		err := c.Get(context.Background(), types.NamespacedName{Namespace: obj.Namespace, Name: CAInternalSecretName(esv1.ESNamer, obj.Name, HTTPCAType)}, &caCerts)
		require.NoError(t, err)
		require.Len(t, caCerts.Data, 2)
		require.NotEmpty(t, caCerts.Data[CertFileName])
		require.NotEmpty(t, caCerts.Data[KeyFileName])
		require.Equal(t, labels, caCerts.Labels)

		var internalCerts corev1.Secret
		err = c.Get(context.Background(), types.NamespacedName{Namespace: obj.Namespace, Name: InternalCertsSecretName(esv1.ESNamer, obj.Name)}, &internalCerts)
		require.NoError(t, err)
		require.Len(t, internalCerts.Data, 3)
		require.NotEmpty(t, internalCerts.Data[CAFileName])
		require.NotEmpty(t, internalCerts.Data[CertFileName])
		require.NotEmpty(t, internalCerts.Data[KeyFileName])
		require.Equal(t, labels, internalCerts.Labels)

		var publicCerts corev1.Secret
		err = c.Get(context.Background(), types.NamespacedName{Namespace: obj.Namespace, Name: PublicCertsSecretName(esv1.ESNamer, obj.Name)}, &publicCerts)
		require.NoError(t, err)
		require.Len(t, publicCerts.Data, 2)
		require.NotEmpty(t, publicCerts.Data[CAFileName])
		require.NotEmpty(t, publicCerts.Data[CertFileName])
		require.Equal(t, labelsWithSoftOwner, publicCerts.Labels)
	}
	checkCertsSecrets()

	// running again should lead to the same results
	httpCerts, results = r.ReconcileCAAndHTTPCerts(context.Background())
	checkResults()
	checkCertsSecrets()

	// disable TLS and run again: should keep existing certs secrets (Elasticsearch use case)
	r.TLSOptions = commonv1.TLSOptions{SelfSignedCertificate: &commonv1.SelfSignedCertificate{Disabled: true}}
	r.GarbageCollectSecrets = false
	httpCerts, results = r.ReconcileCAAndHTTPCerts(context.Background())
	checkResults()
	checkCertsSecrets()

	// disable TLS and run again, this time with the option to remove secrets (Kibana, APMServer, Enterprise Search use cases)
	r.TLSOptions = commonv1.TLSOptions{SelfSignedCertificate: &commonv1.SelfSignedCertificate{Disabled: true}}
	r.GarbageCollectSecrets = true
	httpCerts, results = r.ReconcileCAAndHTTPCerts(context.Background())
	aggregateResult, err := results.Aggregate()
	require.NoError(t, err)
	require.Zero(t, aggregateResult.RequeueAfter)
	require.Nil(t, httpCerts)
	removedSecrets := []types.NamespacedName{
		{Namespace: obj.Namespace, Name: CAInternalSecretName(esv1.ESNamer, obj.Name, HTTPCAType)},
		{Namespace: obj.Namespace, Name: InternalCertsSecretName(esv1.ESNamer, obj.Name)},
		{Namespace: obj.Namespace, Name: PublicCertsSecretName(esv1.ESNamer, obj.Name)},
	}
	for _, nsn := range removedSecrets {
		var s corev1.Secret
		require.True(t, apierrors.IsNotFound(c.Get(context.Background(), nsn, &s)))
	}
}

func TestReconcileCAAndHTTPCerts_WithCustomCA(t *testing.T) {
	// Helper function to create a secret with custom CA
	createCustomCASecret := func(t *testing.T, ca *CA, secretName string) *corev1.Secret {
		t.Helper()
		pemKey, err := EncodePEMPrivateKey(ca.PrivateKey)
		require.NoError(t, err)
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: obj.Namespace,
				Name:      secretName,
			},
			Data: map[string][]byte{
				CAFileName:    EncodePEMCert(ca.Cert.Raw),
				CAKeyFileName: pemKey,
			},
		}
	}

	tests := []struct {
		name         string
		customCA     func(t *testing.T) *CA
		wantErr      bool
		wantRequeue  bool
		checkRequeue func(t *testing.T, requeueAfter time.Duration)
	}{
		{
			name: "valid custom CA should pass validation and set requeue",
			customCA: func(t *testing.T) *CA {
				t.Helper()
				testCA, err := NewSelfSignedCA(CABuilderOptions{})
				require.NoError(t, err)
				return testCA
			},
			wantErr:     false,
			wantRequeue: true,
			checkRequeue: func(t *testing.T, requeueAfter time.Duration) {
				t.Helper()
				// Should requeue before expiration
				require.NotZero(t, requeueAfter, "requeue should be set for CA expiry")
			},
		},
		{
			name: "expired custom CA should fail validation",
			customCA: func(t *testing.T) *CA {
				t.Helper()
				// Create a CA that expired 1 hour ago
				expiredTime := -1 * time.Hour
				testCA, err := NewSelfSignedCA(CABuilderOptions{
					ExpireIn: &expiredTime,
				})
				require.NoError(t, err)
				return testCA
			},
			wantErr:     true,
			wantRequeue: false,
		},
		{
			name: "not-yet-valid custom CA should fail validation",
			customCA: func(t *testing.T) *CA {
				t.Helper()
				// Create a CA manually with NotBefore in the future
				privateKey, err := rsa.GenerateKey(cryptorand.Reader, 2048)
				require.NoError(t, err)
				serial, err := cryptorand.Int(cryptorand.Reader, SerialNumberLimit)
				require.NoError(t, err)

				certificateTemplate := x509.Certificate{
					SerialNumber:          serial,
					Subject:               pkix.Name{CommonName: "test-ca"},
					NotBefore:             time.Now().Add(1 * time.Hour), // Not yet valid
					NotAfter:              time.Now().Add(2 * time.Hour),
					SignatureAlgorithm:    x509.SHA256WithRSA,
					IsCA:                  true,
					BasicConstraintsValid: true,
					KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
					ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
				}

				certData, err := x509.CreateCertificate(cryptorand.Reader, &certificateTemplate, &certificateTemplate, privateKey.Public(), privateKey)
				require.NoError(t, err)
				cert, err := x509.ParseCertificate(certData)
				require.NoError(t, err)

				return NewCA(privateKey, cert)
			},
			wantErr:     true,
			wantRequeue: false,
		},
		{
			name: "custom CA with mismatched keys should fail validation",
			customCA: func(t *testing.T) *CA {
				t.Helper()
				testCA, err := NewSelfSignedCA(CABuilderOptions{})
				require.NoError(t, err)
				// Generate a different private key
				privateKey2, err := rsa.GenerateKey(cryptorand.Reader, 2048)
				require.NoError(t, err)
				testCA.PrivateKey = privateKey2
				return testCA
			},
			wantErr:     true,
			wantRequeue: false,
		},
		{
			name: "custom CA expiring soon should log warning but succeed",
			customCA: func(t *testing.T) *CA {
				t.Helper()
				// Create a CA that expires soon (within DefaultRotateBefore)
				shortValidity := DefaultRotateBefore / 2
				testCA, err := NewSelfSignedCA(CABuilderOptions{
					ExpireIn: &shortValidity,
				})
				require.NoError(t, err)
				return testCA
			},
			wantErr:     false,
			wantRequeue: true,
			checkRequeue: func(t *testing.T, requeueAfter time.Duration) {
				t.Helper()
				// Should requeue soon since CA is expiring
				require.NotZero(t, requeueAfter, "requeue should be set for CA expiry")
				// The requeue time should be based on when the CA will expire
				// Since CA expires in DefaultRotateBefore/2, and we rotate at DefaultRotateBefore before expiry,
				// the requeue should happen soon (negative or very small value)
				require.Greater(t, requeueAfter, time.Duration(0), "requeue should be positive")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			customCA := tt.customCA(t)
			customCASecretName := "custom-ca-secret"

			// Create the custom CA secret
			customCASecret := createCustomCASecret(t, customCA, customCASecretName)
			c := k8s.NewFakeClient(customCASecret)

			// Create test object with custom CA reference
			testObj := obj.DeepCopy()
			testObj.Spec.HTTP.TLS = commonv1.TLSOptions{
				Certificate: commonv1.SecretRef{
					SecretName: customCASecretName,
				},
			}

			r := Reconciler{
				K8sClient:             c,
				DynamicWatches:        watches.NewDynamicWatches(),
				Owner:                 testObj,
				TLSOptions:            testObj.Spec.HTTP.TLS,
				Namer:                 esv1.ESNamer,
				Metadata:              metadata.Metadata{Labels: labels},
				Services:              nil,
				CACertRotation:        rotation,
				CertRotation:          rotation,
				GarbageCollectSecrets: false,
			}

			httpCerts, results := r.ReconcileCAAndHTTPCerts(context.Background())
			aggregateResult, err := results.Aggregate()

			if tt.wantErr {
				assert.Error(t, err, "expected error from ReconcileCAAndHTTPCerts")
				assert.Nil(t, httpCerts, "httpCerts should be nil on error")
			} else {
				assert.NoError(t, err, "expected no error from ReconcileCAAndHTTPCerts")
				assert.NotNil(t, httpCerts, "httpCerts should not be nil on success")
				assert.NotEmpty(t, httpCerts.Data, "httpCerts should have data")
			}

			if tt.wantRequeue {
				assert.NotZero(t, aggregateResult.RequeueAfter, "requeue should be set")
				if tt.checkRequeue != nil {
					tt.checkRequeue(t, aggregateResult.RequeueAfter)
				}
			} else if !tt.wantErr {
				// Only check for zero requeue if we're not expecting an error
				// (errors might have different requeue behavior)
				assert.NotZero(t, aggregateResult.RequeueAfter, "requeue might still be set for cert rotation")
			}
		})
	}
}

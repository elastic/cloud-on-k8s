// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package association

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptorand "crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	toolsevents "k8s.io/client-go/tools/events"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/rolemapping"
)

func TestPKIRequestFingerprintTracksKeyRotation(t *testing.T) {
	kb := sampleKibanaWithESRef()
	es := sampleESWithMTLS()
	r := testReconciler(&kb, &es)
	association := kb.EsAssociation()
	var previous string
	for range 2 {
		managed, results := ReconcileManagedClientCert(t.Context(), r.Client, association, metadata.Metadata{}, "client-cert", nil)
		require.False(t, results.HasError())
		name := managed.Name
		managedCert, err := certificates.GetPrimaryCertificate(managed.CertPem())
		require.NoError(t, err)
		_, err = reconcilePKIRoleMappingRequest(t.Context(), r.Client, association, metadata.Metadata{}, "kb-es", managedCert, "kibana_system", es)
		require.NoError(t, err)
		var request, secret corev1.Secret
		require.NoError(t, r.Get(t.Context(), PKIRoleMappingRequestKey(association, es.Namespace, "kb-es"), &request))
		require.NoError(t, r.Get(t.Context(), types.NamespacedName{Namespace: kb.Namespace, Name: name}, &secret))
		cert, err := certificates.GetPrimaryCertificate(secret.Data[certificates.CertFileName])
		require.NoError(t, err)
		fingerprint := string(request.Data[rolemapping.PublicKeyFingerprintField])
		require.Equal(t, certificates.PublicKeyFingerprint(cert), fingerprint)
		require.NotEqual(t, previous, fingerprint)
		previous = fingerprint
		require.NoError(t, r.Delete(t.Context(), &secret))
	}
}

func TestPKIRequestFingerprintFromUserProvidedClientCert(t *testing.T) {
	kb := sampleKibanaWithESRef()
	es := sampleESWithMTLS()
	r := testReconciler(&kb, &es)
	association := kb.EsAssociation()
	const userSecretName = "user-client-cert"

	var previous string
	for range 2 {
		// generate a fresh certificate and store it as the user-provided Secret
		generated, results := ReconcileManagedClientCert(t.Context(), r.Client, association, metadata.Metadata{}, "generated", nil)
		require.False(t, results.HasError())
		var generatedSecret corev1.Secret
		require.NoError(t, r.Get(t.Context(), types.NamespacedName{Namespace: kb.Namespace, Name: generated.Name}, &generatedSecret))
		userSecret := corev1.Secret{
			Namespace: kb.Namespace, Name: userSecretName,
			Data: generatedSecret.Data,
		}
		require.NoError(t, r.Delete(t.Context(), &generatedSecret))
		require.NoError(t, r.Client.Create(t.Context(), &userSecret))

		copied, err := ReconcileUserProvidedClientCert(t.Context(), r.Client, association, metadata.Metadata{}, "managed-copy", userSecretName, nil)
		require.NoError(t, err)
		certPEM := copied.CertPem()
		require.Equal(t, userSecret.Data[certificates.CertFileName], certPEM)

		copiedCert, err := certificates.GetPrimaryCertificate(certPEM)
		require.NoError(t, err)
		_, err = reconcilePKIRoleMappingRequest(t.Context(), r.Client, association, metadata.Metadata{}, "kb-es", copiedCert, "kibana_system", es)
		require.NoError(t, err)

		var request corev1.Secret
		require.NoError(t, r.Get(t.Context(), PKIRoleMappingRequestKey(association, es.Namespace, "kb-es"), &request))
		cert, err := certificates.GetPrimaryCertificate(userSecret.Data[certificates.CertFileName])
		require.NoError(t, err)
		fingerprint := string(request.Data[rolemapping.PublicKeyFingerprintField])
		require.Equal(t, certificates.PublicKeyFingerprint(cert), fingerprint)
		// the user rotates the certificate: the request follows
		require.NotEqual(t, previous, fingerprint)
		previous = fingerprint

		require.NoError(t, r.Delete(t.Context(), &userSecret))
	}
}

func TestReconcilePKIAuth_ClientCertValidation(t *testing.T) {
	const userSecretName = "user-client-cert"

	tests := []struct {
		name               string
		certPEM            []byte
		wantErr            bool
		wantStatus         commonv1.AssociationStatus
		wantAuthSecretName string
		wantEvent          string
		wantRequest        bool
	}{
		{
			name:       "unparseable certificate",
			certPEM:    []byte("not a certificate"),
			wantErr:    true,
			wantStatus: commonv1.AssociationPending,
		},
		{
			name:       "certificate without a Subject CN (SAN-only)",
			certPEM:    selfSignedCertPEM(t, pkix.Name{}),
			wantStatus: commonv1.AssociationPending,
			wantEvent:  "PKI authentication requires the client certificate in Secret kbns/" + userSecretName + " to have a non-empty Subject Common Name",
		},
		{
			name:       "certificate with a blank Subject CN",
			certPEM:    selfSignedCertPEM(t, pkix.Name{CommonName: " ", Organization: []string{"elastic"}}),
			wantStatus: commonv1.AssociationPending,
			wantEvent:  "non-empty Subject Common Name",
		},
		{
			name:               "certificate with a Subject CN",
			certPEM:            selfSignedCertPEM(t, pkix.Name{CommonName: "kibana"}),
			wantAuthSecretName: commonv1.NoAuthRequiredValue,
			wantRequest:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kb := sampleKibanaWithESRef()
			kb.Spec.ElasticsearchRef.AuthenticationMode = commonv1.AuthenticationModePKI
			kb.Spec.ElasticsearchRef.ClientCertificateSecretName = userSecretName
			es := sampleESWithMTLSAndPKIVersion()
			r := testReconciler(&kb, &es)
			association := kb.EsAssociation()
			clientCertSecret := &certificates.CertificatesSecret{
				Namespace: kb.Namespace, Name: "managed-copy",
				Data: map[string][]byte{certificates.CertFileName: tt.certPEM}}

			ac, status, err := r.reconcilePKIAuth(t.Context(), association, es, metadata.Metadata{}, true, clientCertSecret, es.Status.Version, "kibana_system")

			require.Equal(t, tt.wantErr, err != nil, "unexpected error: %v", err)
			require.Equal(t, tt.wantStatus, status)
			require.Equal(t, tt.wantAuthSecretName, ac.secretName)
			fakeRec, ok := r.recorder.(*toolsevents.FakeRecorder)
			require.True(t, ok, "recorder must be a FakeRecorder")
			event := fetchEvent(fakeRec)
			require.Equal(t, tt.wantEvent != "", strings.HasPrefix(event, corev1.EventTypeWarning), "unexpected event: %q", event)
			require.Contains(t, event, tt.wantEvent)
			getErr := r.Get(t.Context(), PKIRoleMappingRequestKey(association, es.Namespace, kbAssociationInfo.AssociationName), &corev1.Secret{})
			require.Equal(t, !tt.wantRequest, apierrors.IsNotFound(getErr), "unexpected request Secret lookup result: %v", getErr)
		})
	}
}

// selfSignedCertPEM returns a PEM-encoded self-signed client certificate with the given subject.
func selfSignedCertPEM(t *testing.T, subject pkix.Name) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), cryptorand.Reader)
	require.NoError(t, err)
	return selfSignedCertForKey(t, key, subject)
}

// selfSignedCertForKey returns a PEM-encoded self-signed client certificate for the given key and subject.
func selfSignedCertForKey(t *testing.T, key crypto.Signer, subject pkix.Name) []byte {
	t.Helper()
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      subject,
		DNSNames:     []string{"client.example.com"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(cryptorand.Reader, &template, &template, key.Public(), key)
	require.NoError(t, err)
	return certificates.EncodePEMCert(der)
}

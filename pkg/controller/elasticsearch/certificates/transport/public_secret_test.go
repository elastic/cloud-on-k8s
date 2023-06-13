// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package transport

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func TestReconcileTransportCertsPublicSecret(t *testing.T) {
	owner := &esv1.Elasticsearch{
		ObjectMeta: v1.ObjectMeta{Name: "test-es-name", Namespace: "test-namespace"},
		TypeMeta:   v1.TypeMeta{Kind: esv1.Kind},
	}

	ca := genCA(t)

	namespacedSecretName := PublicCertsSecretRef(k8s.ExtractNamespacedName(owner))

	mkClient := func(t *testing.T, objs ...client.Object) k8s.Client {
		t.Helper()
		return k8s.NewFakeClient(objs...)
	}

	mkWantedSecret := func(t *testing.T) *corev1.Secret {
		t.Helper()
		meta := k8s.ToObjectMeta(namespacedSecretName)
		labels := label.NewLabels(k8s.ExtractNamespacedName(owner))
		labels[reconciler.SoftOwnerKindLabel] = owner.Kind
		labels[reconciler.SoftOwnerNameLabel] = owner.Name
		labels[reconciler.SoftOwnerNamespaceLabel] = owner.Namespace

		meta.SetLabels(labels)

		wantSecret := &corev1.Secret{
			ObjectMeta: meta,
			Data: map[string][]byte{
				certificates.CAFileName: certificates.EncodePEMCert(ca.Cert.Raw),
			},
		}
		return wantSecret
	}

	tests := []struct {
		name       string
		client     func(*testing.T, ...client.Object) k8s.Client
		extraCA    []byte
		wantSecret func(*testing.T) *corev1.Secret
		wantErr    bool
	}{
		{
			name:       "is created if missing",
			client:     mkClient,
			wantSecret: mkWantedSecret,
		},
		{
			name: "is updated on mismatch",
			client: func(t *testing.T, _ ...client.Object) k8s.Client {
				t.Helper()
				s := mkWantedSecret(t)
				s.Data[certificates.CAFileName] = []byte("/some/ca.crt")
				return mkClient(t, s)
			},
			wantSecret: mkWantedSecret,
		},
		{
			name:    "concatenates multiple CAs",
			client:  mkClient,
			extraCA: extraCA,
			wantSecret: func(t *testing.T) *corev1.Secret {
				t.Helper()
				s := mkWantedSecret(t)
				s.Data[certificates.CAFileName] = bytes.Join([][]byte{s.Data[certificates.CAFileName], extraCA}, nil)
				return s
			},
			wantErr: false,
		},
		{
			name: "removes extraneous keys",
			client: func(t *testing.T, _ ...client.Object) k8s.Client {
				t.Helper()
				s := mkWantedSecret(t)
				s.Data["extra"] = []byte{0, 1, 2, 3}
				return mkClient(t, s)
			},
			wantSecret: mkWantedSecret,
		},
		{
			name: "preserves labels and annotations",
			client: func(t *testing.T, _ ...client.Object) k8s.Client {
				t.Helper()
				s := mkWantedSecret(t)
				if s.Labels == nil {
					s.Labels = make(map[string]string)
				}
				s.Labels["label1"] = "labelValue1"
				s.Labels["label2"] = "labelValue2"
				if s.Annotations == nil {
					s.Annotations = make(map[string]string)
				}
				s.Annotations["annotation1"] = "annotationValue1"
				s.Annotations["annotation2"] = "annotationValue2"
				return mkClient(t, s)
			},
			wantSecret: func(t *testing.T) *corev1.Secret {
				t.Helper()
				s := mkWantedSecret(t)
				if s.Labels == nil {
					s.Labels = make(map[string]string)
				}
				s.Labels["label1"] = "labelValue1"
				s.Labels["label2"] = "labelValue2"
				if s.Annotations == nil {
					s.Annotations = make(map[string]string)
				}
				s.Annotations["annotation1"] = "annotationValue1"
				s.Annotations["annotation2"] = "annotationValue2"
				return s
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			client := tt.client(t)
			err := ReconcileTransportCertsPublicSecret(context.Background(), client, *owner, ca, tt.extraCA)
			if tt.wantErr {
				require.Error(t, err, "Failed to reconcile")
				return
			}

			var gotSecret corev1.Secret
			err = client.Get(context.Background(), namespacedSecretName, &gotSecret)
			require.NoError(t, err, "Failed to get secret")

			wantSecret := tt.wantSecret(t)
			comparison.AssertEqual(t, wantSecret, &gotSecret)
		})
	}
}

func genCA(t *testing.T) *certificates.CA {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA private key: %v", err)
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		t.Fatalf("failed to generate serial number: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Elastic"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"test-es"},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	cert, err := x509.ParseCertificate(derBytes)
	if err != nil {
		t.Fatalf("failed to parse certificate: %v", err)
	}

	return &certificates.CA{
		PrivateKey: priv,
		Cert:       cert,
	}
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package transport

import (
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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func TestReconcileTransportCertsPublicSecret(t *testing.T) {
	owner := &esv1.Elasticsearch{
		ObjectMeta: v1.ObjectMeta{Name: "test-es-name", Namespace: "test-namespace"},
	}

	ca := genCA(t)

	namespacedSecretName := PublicCertsSecretRef(k8s.ExtractNamespacedName(owner))

	mkClient := func(t *testing.T, objs ...runtime.Object) k8s.Client {
		t.Helper()
		return k8s.WrappedFakeClient(objs...)
	}

	mkWantedSecret := func(t *testing.T) *corev1.Secret {
		t.Helper()
		meta := k8s.ToObjectMeta(namespacedSecretName)
		meta.SetLabels(label.NewLabels(k8s.ExtractNamespacedName(owner)))

		wantSecret := &corev1.Secret{
			ObjectMeta: meta,
			Data: map[string][]byte{
				certificates.CAFileName: certificates.EncodePEMCert(ca.Cert.Raw),
			},
		}

		if err := controllerutil.SetControllerReference(owner, wantSecret, scheme.Scheme); err != nil {
			t.Fatal(err)
		}

		return wantSecret
	}

	tests := []struct {
		name       string
		client     func(*testing.T, ...runtime.Object) k8s.Client
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
			client: func(t *testing.T, _ ...runtime.Object) k8s.Client {
				s := mkWantedSecret(t)
				s.Data[certificates.CAFileName] = []byte("/some/ca.crt")
				return mkClient(t, s)
			},
			wantSecret: mkWantedSecret,
		},
		{
			name: "removes extraneous keys",
			client: func(t *testing.T, _ ...runtime.Object) k8s.Client {
				s := mkWantedSecret(t)
				s.Data["extra"] = []byte{0, 1, 2, 3}
				return mkClient(t, s)
			},
			wantSecret: mkWantedSecret,
		},
		{
			name: "preserves labels and annotations",
			client: func(t *testing.T, _ ...runtime.Object) k8s.Client {
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
			err := ReconcileTransportCertsPublicSecret(client, *owner, ca)
			if tt.wantErr {
				require.Error(t, err, "Failed to reconcile")
				return
			}

			var gotSecret corev1.Secret
			err = client.Get(namespacedSecretName, &gotSecret)
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

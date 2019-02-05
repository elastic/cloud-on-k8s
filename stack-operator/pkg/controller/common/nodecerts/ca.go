package nodecerts

import (
	"bytes"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"

	"github.com/elastic/k8s-operators/stack-operator/pkg/controller/common/reconciler"

	"github.com/elastic/k8s-operators/stack-operator/pkg/utils/k8s"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
)

var (
	// SerialNumberLimit is the maximum number used as a certificate serial number
	SerialNumberLimit = new(big.Int).Lsh(big.NewInt(1), 128)
)

// Ca is a simple certificate authority
type Ca struct {
	// privateKey is the CA private key
	privateKey *rsa.PrivateKey
	// Cert is the certificate used to issue new certificates
	Cert *x509.Certificate
}

// ValidatedCertificateTemplate is a type alias used to convey that the certificate template has been validated and
// should be considered trusted.
type ValidatedCertificateTemplate x509.Certificate

// NewSelfSignedCa creates a new Ca that uses a self-signed certificate.
func NewSelfSignedCa(cn string) (*Ca, error) {
	// TODO: constructor that takes the key?
	key, err := rsa.GenerateKey(cryptorand.Reader, 2048)
	if err != nil {
		return nil, errors.Wrap(err, "unable to generate the private key")
	}
	return NewSelfSignedCaUsingKey(cn, key)
}

// NewSelfSignedCaUsingKey creates a new Ca that uses a self-signed certificate using the provided private key
func NewSelfSignedCaUsingKey(cn string, key *rsa.PrivateKey) (*Ca, error) {
	// create a self-signed certificate for ourselves:

	// generate a serial number
	serial, err := cryptorand.Int(cryptorand.Reader, SerialNumberLimit)
	if err != nil {
		return nil, err
	}

	certificateTemplate := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:         cn,
			OrganizationalUnit: []string{rand.String(16)},
		},
		NotBefore:             time.Now().Add(-1 * time.Minute),
		NotAfter:              time.Now().Add(24 * 365 * time.Hour),
		SignatureAlgorithm:    x509.SHA256WithRSA,
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}

	certData, err := x509.CreateCertificate(cryptorand.Reader, &certificateTemplate, &certificateTemplate, key.Public(), key)
	if err != nil {
		return nil, err
	}

	cert, err := x509.ParseCertificate(certData)
	if err != nil {
		return nil, err
	}

	return &Ca{
		privateKey: key,
		Cert:       cert,
	}, nil
}

// CreateCertificate signs and creates a new certificate for a validated template.
func (c *Ca) CreateCertificate(
	validatedCertificateTemplate ValidatedCertificateTemplate,
) ([]byte, error) {
	// generate a serial number
	serial, err := cryptorand.Int(cryptorand.Reader, SerialNumberLimit)
	if err != nil {
		return nil, errors.Wrap(err, "unable to generate serial number for new certificate")
	}
	validatedCertificateTemplate.SerialNumber = serial
	validatedCertificateTemplate.Issuer = c.Cert.Issuer

	certTemplate := x509.Certificate(validatedCertificateTemplate)

	certData, err := x509.CreateCertificate(
		cryptorand.Reader,
		&certTemplate,
		c.Cert,
		validatedCertificateTemplate.PublicKey,
		c.privateKey,
	)

	return certData, err
}

// ReconcilePublicCertsSecret ensures that a secret containing the Ca's certificate as `ca.pem` exists as the specified
// objectKey
func (c *Ca) ReconcilePublicCertsSecret(
	cl k8s.Client,
	objectKey types.NamespacedName,
	owner metav1.Object,
	scheme *runtime.Scheme,
) error {
	// TODO: how to do rotation of certs here? cross signing possible, likely not.
	expectedCaKeyBytes := pem.EncodeToMemory(&pem.Block{Type: BlockTypeCertificate, Bytes: c.Cert.Raw})

	clusterCASecret := corev1.Secret{
		ObjectMeta: k8s.ToObjectMeta(objectKey),
		Data: map[string][]byte{
			SecretCAKey: expectedCaKeyBytes,
		},
	}

	reconciled := &corev1.Secret{}
	return reconciler.ReconcileResource(reconciler.Params{
		Client:     cl,
		Scheme:     scheme,
		Owner:      owner,
		Expected:   &clusterCASecret,
		Reconciled: reconciled,
		NeedsUpdate: func() bool {
			// if Data is nil, create it in case we're starting with a poorly initialized secret
			if reconciled.Data == nil {
				reconciled.Data = make(map[string][]byte)
			}
			caKey, ok := reconciled.Data[SecretCAKey]
			return !ok || !bytes.Equal(caKey, expectedCaKeyBytes)

		},
		UpdateReconciled: func() {
			reconciled.Data[SecretCAKey] = expectedCaKeyBytes
		},
	})

}

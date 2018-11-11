package nodecerts

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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

// CreateCertificateForValidatedCertificateTemplate signs and creates a new certificate
// Important note: the certificate template is assumed to have passed validation.
func (c *Ca) CreateCertificateForValidatedCertificateTemplate(
	validatedCertificateTemplate x509.Certificate,
) ([]byte, error) {
	// generate a serial number
	serial, err := cryptorand.Int(cryptorand.Reader, SerialNumberLimit)
	if err != nil {
		return nil, errors.Wrap(err, "unable to generate serial number for new certificate")
	}
	validatedCertificateTemplate.SerialNumber = serial
	validatedCertificateTemplate.Issuer = c.Cert.Issuer

	certData, err := x509.CreateCertificate(
		cryptorand.Reader,
		&validatedCertificateTemplate,
		c.Cert,
		validatedCertificateTemplate.PublicKey,
		c.privateKey,
	)

	return certData, err
}

// ReconcilePublicCertsSecret ensures that a secret containing the Ca's certificate as `ca.pem` exists as the specified
// objectKey
func (c *Ca) ReconcilePublicCertsSecret(
	cl client.Client,
	objectKey types.NamespacedName,
	owner v1.Object,
	scheme *runtime.Scheme,
) error {
	// TODO: how to do rotation of certs here? cross signing possible, likely not.
	expectedCaKeyBytes := pem.EncodeToMemory(&pem.Block{Type: BlockTypeCertificate, Bytes: c.Cert.Raw})

	var clusterCASecret corev1.Secret
	if err := cl.Get(context.TODO(), objectKey, &clusterCASecret); err != nil && !apierrors.IsNotFound(err) {
		return err
	} else if apierrors.IsNotFound(err) {
		clusterCASecret = corev1.Secret{
			ObjectMeta: v1.ObjectMeta{
				Name:      objectKey.Name,
				Namespace: objectKey.Namespace,
			},
			Data: map[string][]byte{
				SecretCAKey: expectedCaKeyBytes,
			},
		}

		if err := controllerutil.SetControllerReference(owner, &clusterCASecret, scheme); err != nil {
			return err
		}

		log.Info(fmt.Sprintf(
			"Creating CA public certs for secret %s in namespace %s", objectKey.Name, objectKey.Namespace,
		))
		return cl.Create(context.TODO(), &clusterCASecret)
	}

	// if Data is nil, create it in case we're starting with a poorly initialized secret
	if clusterCASecret.Data == nil {
		clusterCASecret.Data = make(map[string][]byte)
	}

	// if they secret does not contain our cert, update it
	if caKey, ok := clusterCASecret.Data[SecretCAKey]; !ok || !bytes.Equal(caKey, expectedCaKeyBytes) {
		clusterCASecret.Data[SecretCAKey] = expectedCaKeyBytes

		log.Info(fmt.Sprintf(
			"Updating CA public certs for secret %s in namespace %s", objectKey.Name, objectKey.Namespace,
		))
		return cl.Update(context.TODO(), &clusterCASecret)
	}

	return nil
}

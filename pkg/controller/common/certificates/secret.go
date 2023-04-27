// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package certificates

import (
	"context"
	"errors"
	"fmt"

	pkgerrors "github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

const (
	// CAFileName is used for the CA Certificates inside a secret.
	CAFileName = "ca.crt"
	// CAKeyFileName is used for the CA certificate's private key inside a secret.
	CAKeyFileName = "ca.key"
	// CertFileName is used for Certificates inside a secret.
	CertFileName = "tls.crt"
	// KeyFileName is used for Private Keys inside a secret.
	KeyFileName = "tls.key"

	// certificate secrets suffixes
	certsPublicSecretName   = "certs-public"
	certsInternalSecretName = "certs-internal"

	// http certs volume
	HTTPCertificatesSecretVolumeName      = "elastic-internal-http-certificates"
	HTTPCertificatesSecretVolumeMountPath = "/mnt/elastic-internal/http-certs" //nolint:gosec
)

func InternalCertsSecretName(namer name.Namer, ownerName string) string {
	return namer.Suffix(ownerName, string(HTTPCAType), certsInternalSecretName)
}

func PublicCertsSecretName(namer name.Namer, ownerName string) string {
	return namer.Suffix(ownerName, string(HTTPCAType), certsPublicSecretName)
}

func PublicTransportCertsSecretName(namer name.Namer, ownerName string) string {
	return namer.Suffix(ownerName, string(TransportCAType), certsPublicSecretName)
}

// PublicCertsSecretRef returns the NamespacedName for the Secret containing the publicly available HTTP CA.
func PublicCertsSecretRef(namer name.Namer, es types.NamespacedName) types.NamespacedName {
	return types.NamespacedName{
		Name:      PublicCertsSecretName(namer, es.Name),
		Namespace: es.Namespace,
	}
}

// HTTPCertSecretVolume returns a SecretVolume to hold the HTTP certs for the given resource.
func HTTPCertSecretVolume(namer name.Namer, name string) volume.SecretVolume {
	return volume.NewSecretVolumeWithMountPath(
		InternalCertsSecretName(namer, name),
		HTTPCertificatesSecretVolumeName,
		HTTPCertificatesSecretVolumeMountPath,
	)
}

type CertificatesSecret struct { //nolint:revive
	v1.Secret
	ca *CA
}

func NewCertificatesSecret(secret v1.Secret) (*CertificatesSecret, error) {
	result := CertificatesSecret{Secret: secret}
	if err := result.parse(); err != nil {
		return nil, err
	}
	return &result, nil
}

// HasCA returns true if this secret has a CA certificate.
func (s *CertificatesSecret) HasCA() bool {
	if s == nil {
		return false
	}
	bytes, exists := s.Data[CAFileName]
	return exists && len(bytes) > 0
}

// CAPem returns the certificate of the certificate authority.
func (s *CertificatesSecret) CAPem() []byte {
	return s.Data[CAFileName]
}

// CA returns a pointer to the in-memory representation of the CA contained in the secret.
func (s *CertificatesSecret) CA() *CA {
	return s.ca
}

// CertChain combines the certificate of the CA and the host certificate.
func (s *CertificatesSecret) CertChain() []byte {
	return append(s.CertPem(), s.CAPem()...)
}

func (s *CertificatesSecret) CertPem() []byte {
	return s.Data[CertFileName]
}

func (s *CertificatesSecret) KeyPem() []byte {
	return s.Data[KeyFileName]
}

func (s *CertificatesSecret) HasCAPrivateKey() bool {
	if s == nil {
		return false
	}
	// the presence of the key means by implication that we have a full CA, validation ensures that the CA cert exists
	_, exists := s.Data[CAKeyFileName]
	return exists
}

func (s *CertificatesSecret) HasLeafCertificate() bool {
	return s != nil && !s.HasCAPrivateKey()
}

func (s *CertificatesSecret) parseCustomCA() error {
	// flag up user error when specifying both CA certificate with key and leaf certificate
	_, tlsKeyExists := s.Data[KeyFileName]
	_, tlsCertExists := s.Data[CertFileName]
	if tlsKeyExists || tlsCertExists {
		return fmt.Errorf("cannot specify %s or %s when %s is set in %s/%s",
			KeyFileName, CertFileName, CAKeyFileName, s.Namespace, s.Name)
	}
	ca, err := parseCAFromSecret(s.Secret, CAKeyFileName, CAFileName)
	if err == nil {
		// breaking the validation contract here by remembering the results to avoid parsing everything once more
		s.ca = ca
	}
	return err
}

// parse checks that mandatory fields are present.
// It does not check that the public key matches the private key.
func (s *CertificatesSecret) parse() error {
	if s.HasCAPrivateKey() {
		return s.parseCustomCA()
	}

	// Validate private key
	key, exist := s.Data[KeyFileName]
	if !exist {
		return pkgerrors.Errorf("can't find private key %s in %s/%s", KeyFileName, s.Namespace, s.Name)
	}
	_, err := ParsePEMPrivateKey(key)
	if err != nil && !errors.Is(err, ErrEncryptedPrivateKey) {
		return err
	}
	// Validate host certificate
	cert, exist := s.Data[CertFileName]
	if !exist {
		return pkgerrors.Errorf("can't find certificate %s in %s/%s", CertFileName, s.Namespace, s.Name)
	}
	_, err = ParsePEMCerts(cert)
	if err != nil {
		return err
	}
	// Eventually validate CA certificate
	ca, exist := s.Data[CAFileName]
	if !exist {
		return nil
	}
	_, err = ParsePEMCerts(ca)
	if err != nil {
		return err
	}
	return nil
}

func GetSecretFromRef(c k8s.Client, owner types.NamespacedName, secretRef commonv1.SecretRef) (*v1.Secret, error) {
	secretName := secretRef.SecretName
	if secretName == "" {
		return nil, nil
	}

	var secret v1.Secret
	if err := c.Get(context.Background(), types.NamespacedName{Name: secretName, Namespace: owner.Namespace}, &secret); err != nil {
		return nil, err
	}
	return &secret, nil
}

// validCustomCertificatesOrNil returns the custom certificates to use or nil if there is none specified
func validCustomCertificatesOrNil(
	c k8s.Client,
	owner types.NamespacedName,
	tls commonv1.TLSOptions,
) (*CertificatesSecret, error) {
	secret, err := GetSecretFromRef(c, owner, tls.Certificate)
	if err != nil || secret == nil {
		return nil, err
	}
	return NewCertificatesSecret(*secret)
}

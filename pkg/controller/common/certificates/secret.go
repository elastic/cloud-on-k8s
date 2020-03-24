// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package certificates

import (
	pkgerrors "github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

const (
	// CAFileName is used for the CA Certificates inside a secret
	CAFileName = "ca.crt"
	// CertFileName is used for Certificates inside a secret
	CertFileName = "tls.crt"
	// KeyFileName is used for Private Keys inside a secret
	KeyFileName = "tls.key"

	// certificate secrets suffixes
	certsPublicSecretName   = "certs-public"
	certsInternalSecretName = "certs-internal"

	// http certs volume
	HTTPCertificatesSecretVolumeName      = "elastic-internal-http-certificates"
	HTTPCertificatesSecretVolumeMountPath = "/mnt/elastic-internal/http-certs" // nolint
)

func InternalCertsSecretName(namer name.Namer, ownerName string) string {
	return namer.Suffix(ownerName, "http", certsInternalSecretName)
}

func PublicCertsSecretName(namer name.Namer, ownerName string) string {
	return namer.Suffix(ownerName, "http", certsPublicSecretName)
}

func PublicCASecretName(namer name.Namer, ownerName string) string {
	return namer.Suffix(ownerName, "ca", certsPublicSecretName)
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

type CertificatesSecret v1.Secret

// CAPem returns the certificate of the certificate authority.
func (s CertificatesSecret) CAPem() []byte {
	return s.Data[CAFileName]
}

// CertChain combines the certificate of the CA and the host certificate.
func (s CertificatesSecret) CertChain() []byte {
	return append(s.CertPem(), s.CAPem()...)
}

func (s CertificatesSecret) CertPem() []byte {
	return s.Data[CertFileName]
}

func (s CertificatesSecret) KeyPem() []byte {
	return s.Data[KeyFileName]
}

// Validate checks that mandatory fields are present.
// It does not check that the public key matches the private key.
func (s CertificatesSecret) Validate() error {
	// Validate private key
	key, exist := s.Data[KeyFileName]
	if !exist {
		return pkgerrors.Errorf("can't find private key %s in %s/%s", KeyFileName, s.Namespace, s.Name)
	}
	_, err := ParsePEMPrivateKey(key)
	if err != nil {
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

// GetCustomCertificates returns the custom certificates to use or nil if there is none specified
func GetCustomCertificates(
	c k8s.Client,
	owner types.NamespacedName,
	tls commonv1.TLSOptions,
) (*CertificatesSecret, error) {
	secretName := tls.Certificate.SecretName
	if secretName == "" {
		return nil, nil
	}

	var secret v1.Secret
	if err := c.Get(types.NamespacedName{Name: secretName, Namespace: owner.Namespace}, &secret); err != nil {
		return nil, err
	}

	result := CertificatesSecret(secret)

	return &result, nil
}

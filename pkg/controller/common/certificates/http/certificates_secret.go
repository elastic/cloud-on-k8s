// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package http

import (
	pkgerrors "github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates/certutils"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

type CertificatesSecret v1.Secret

// CAPem returns the certificate of the certificate authority.
func (s CertificatesSecret) CAPem() []byte {
	return s.Data[certutils.CAFileName]
}

// CertChain combines the certificate of the CA and the host certificate.
func (s CertificatesSecret) CertChain() []byte {
	return append(s.CertPem(), s.CAPem()...)
}

func (s CertificatesSecret) CertPem() []byte {
	return s.Data[certutils.CertFileName]
}

func (s CertificatesSecret) KeyPem() []byte {
	return s.Data[certutils.KeyFileName]
}

// Validate checks that mandatory fields are present.
// It does not check that the public key matches the private key.
func (s CertificatesSecret) Validate() error {
	// Validate private key
	key, exist := s.Data[certutils.KeyFileName]
	if !exist {
		return pkgerrors.Errorf("can't find private key %s in %s/%s", certutils.KeyFileName, s.Namespace, s.Name)
	}
	_, err := certutils.ParsePEMPrivateKey(key)
	if err != nil {
		return err
	}
	// Validate host certificate
	cert, exist := s.Data[certutils.CertFileName]
	if !exist {
		return pkgerrors.Errorf("can't find certificate %s in %s/%s", certutils.CertFileName, s.Namespace, s.Name)
	}
	_, err = certutils.ParsePEMCerts(cert)
	if err != nil {
		return err
	}
	// Eventually validate CA certificate
	ca, exist := s.Data[certutils.CAFileName]
	if !exist {
		return nil
	}
	_, err = certutils.ParsePEMCerts(ca)
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

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package http

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

type CertificatesSecret v1.Secret

// CAPem returns the certificate of the certificate authority.
func (s CertificatesSecret) CAPem() []byte {
	if ca, exist := s.Data[certificates.CAFileName]; exist {
		return ca
	}
	return nil
}

// CertChain combines the certificate of the CA and the host certificate.
func (s CertificatesSecret) CertChain() []byte {
	return append(s.CertPem(), s.CAPem()...)
}

func (s CertificatesSecret) CertPem() []byte {
	return s.Data[certificates.CertFileName]
}

func (s CertificatesSecret) KeyPem() []byte {
	return s.Data[certificates.KeyFileName]
}

// Validate checks that mandatory fields are present.
// It does not check that the public key matches the private key.
func (s CertificatesSecret) Validate() error {
	// Validate private key
	key, exist := s.Data[certificates.KeyFileName]
	if !exist {
		return fmt.Errorf("can't find private key %s in %s/%s", certificates.KeyFileName, s.Namespace, s.Name)
	}
	_, err := certificates.ParsePEMPrivateKey(key)
	if err != nil {
		return err
	}
	// Validate host certificate
	cert, exist := s.Data[certificates.CertFileName]
	if !exist {
		return fmt.Errorf("can't find certificate %s in %s/%s", certificates.CertFileName, s.Namespace, s.Name)
	}
	_, err = certificates.ParsePEMCerts(cert)
	if err != nil {
		return err
	}
	// Eventually validate CA certificate
	ca, exist := s.Data[certificates.CAFileName]
	if !exist {
		return nil
	}
	_, err = certificates.ParsePEMCerts(ca)
	if err != nil {
		return err
	}
	return nil
}

// GetCustomCertificates returns the custom certificates to use or nil if there is none specified
func GetCustomCertificates(
	c k8s.Client,
	owner types.NamespacedName,
	tls v1alpha1.TLSOptions,
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

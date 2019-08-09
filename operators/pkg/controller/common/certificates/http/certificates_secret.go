// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package http

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

type CertificatesSecret v1.Secret

// CaPem returns the certificate of the certificate authority.
func (s CertificatesSecret) CaPem() []byte {
	if ca, exist := s.Data[certificates.CAFileName]; exist {
		return ca
	}
	return nil
}

// CertChain combines the certificate of the CA and the host certificate.
func (s CertificatesSecret) CertChain() []byte {
	return append(s.CaPem(), s.CertPem()...)
}

func (s CertificatesSecret) CertPem() []byte {
	return s.Data[certificates.CertFileName]
}

func (s CertificatesSecret) KeyPem() []byte {
	return s.Data[certificates.KeyFileName]
}

func (s CertificatesSecret) Validate() error {
	// TODO: Validate that the contents of the secret forms a valid certificate.
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

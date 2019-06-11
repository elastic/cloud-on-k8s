// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package http

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

type CertificatesSecret v1.Secret

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
	es v1alpha1.Elasticsearch,
) (*CertificatesSecret, error) {
	secretName := es.Spec.HTTP.TLS.Certificate.SecretName
	if secretName == "" {
		return nil, nil
	}

	var secret v1.Secret
	if err := c.Get(types.NamespacedName{Name: secretName, Namespace: es.Namespace}, &secret); err != nil {
		return nil, err
	}

	result := CertificatesSecret(secret)

	return &result, nil
}

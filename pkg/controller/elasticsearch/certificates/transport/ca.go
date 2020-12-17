// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package transport

import (
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	pkgerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func CustomTransportCertsWatchKey(es types.NamespacedName) string {
	return esv1.ESNamer.Suffix(es.Name, "custom-transport-certs")
}

func ReconcileOrRetrieveCA(
	driver driver.Interface,
	es esv1.Elasticsearch,
	labels map[string]string,
	rotationParams certificates.RotationParams,
) (*certificates.CA, error) {
	esNSN := k8s.ExtractNamespacedName(&es)

	// Set up a dynamic watch to re-reconcile if users change or recreate the custom certificate secret. But also run this
	// to remove previously created watches if a user removes the custom certificate.
	if err := certificates.ReconcileCustomCertWatch(
		driver.DynamicWatches(),
		CustomTransportCertsWatchKey(esNSN),
		esNSN,
		es.Spec.Transport.TLS.Certificate,
	); err != nil {
		return nil, err
	}

	customCASecret, err := getCustomCACertificates(driver.K8sClient(), esNSN, es.Spec.Transport.TLS)
	if err != nil {
		return nil, err
	}
	// 1. No custom certs are specified reconcile our internal self-signed CA instead (probably the common case)
	if customCASecret == nil {
		return certificates.ReconcileCAForOwner(
			driver.K8sClient(),
			esv1.ESNamer,
			&es,
			labels,
			certificates.TransportCAType,
			rotationParams,
		)
	}

	// 2. Assuming from here on the user wants to use custom certs and has configured a secret with them.

	// Garbage collect the self-signed CA secret which might be left over from an earlier revision on a best effort basis.
	_ = driver.K8sClient().Delete(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      certificates.CAInternalSecretName(esv1.ESNamer, esNSN.Name, certificates.TransportCAType),
			Namespace: esNSN.Namespace,
		},
	})

	// Try to parse the provided secret to get to the CA and to report any validation errors to the user
	ca, err := customCASecret.Parse()
	if err != nil {
		// Surface validation/parsing errors to the user via an event otherwise they might be hard to spot
		// validation at admission would also be an alternative but seems quite costly and secret contents might change
		driver.Recorder().Eventf(&es, corev1.EventTypeWarning, events.EventReasonValidation, err.Error())
	}
	return ca, err
}

type CustomCASecret corev1.Secret

func (s CustomCASecret) CertPem() []byte {
	return s.Data[certificates.CertFileName]
}

func (s CustomCASecret) KeyPem() []byte {
	return s.Data[certificates.KeyFileName]
}

// Parse checks that mandatory fields are present and returns a CA struct.
// It does not check that the public key matches the private key.
func (s CustomCASecret) Parse() (*certificates.CA, error) {
	// Validate private key
	key, exist := s.Data[certificates.KeyFileName]
	if !exist {
		return nil, pkgerrors.Errorf("can't find private key %s in %s/%s", certificates.KeyFileName, s.Namespace, s.Name)
	}
	privateKey, err := certificates.ParsePEMPrivateKey(key)
	if err != nil && !errors.Is(err, certificates.ErrEncryptedPrivateKey) {
		return nil, pkgerrors.Wrapf(err, "can't parse private key  %s in %s/%s", certificates.KeyFileName, s.Namespace, s.Name)
	}
	// Validate CA certificate
	cert, exist := s.Data[certificates.CertFileName]
	if !exist {
		return nil, pkgerrors.Errorf("can't find certificate %s in %s/%s", certificates.CertFileName, s.Namespace, s.Name)
	}
	pubKeys, err := certificates.ParsePEMCerts(cert)
	if err != nil {
		return nil, pkgerrors.Wrapf(err, "can't parse CA certificate %s in %s/%s", certificates.CertFileName, s.Namespace, s.Name)
	}
	if len(pubKeys) != 1 {
		return nil, pkgerrors.Errorf("only expected one PEM formated CA certificate in %s/%s", s.Namespace, s.Name)
	}

	certificate := pubKeys[0]
	if !certificate.IsCA {
		return nil, pkgerrors.Errorf("valid certificate %s found but it is not a CA certificate in %s/%s", certificates.CertFileName, s.Namespace, s.Name)
	}
	return certificates.NewCA(privateKey, certificate), nil
}

// getCustomCACertificates returns the custom certificates to use or nil if there is none specified
func getCustomCACertificates(
	c k8s.Client,
	owner types.NamespacedName,
	tls esv1.TransportTLSOptions,
) (*CustomCASecret, error) {
	secretName := tls.Certificate.SecretName
	if secretName == "" {
		return nil, nil
	}

	var secret corev1.Secret
	if err := c.Get(types.NamespacedName{Name: secretName, Namespace: owner.Namespace}, &secret); err != nil {
		return nil, err
	}

	result := CustomCASecret(secret)

	return &result, nil
}

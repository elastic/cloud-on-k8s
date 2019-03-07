// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package nodecerts

import (
	"errors"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// CAPrivateKeyFileName is the name of the private key section in the CA secret
	CAPrivateKeyFileName = "private.key"
)

// CACertSecretName returns the name of the CA cert secret for the given cluster.
func CACertSecretName(clusterName string) string {
	return clusterName + "-ca"
}

// caPrivateKeySecretName returns the name of the CA private key secret for the given cluster.
func caPrivateKeySecretName(clusterName string) string {
	return clusterName + "-ca-private-key"
}

// caFromSecrets parses the given secrets into a CA.
// It returns false if the secrets could not be parsed into a CA.
func caFromSecrets(certSecret corev1.Secret, privateKeySecret corev1.Secret) (*certificates.CA, bool) {
	if certSecret.Data == nil {
		return nil, false
	}
	caBytes, exists := certSecret.Data[certificates.CAFileName]
	if !exists || len(caBytes) == 0 {
		return nil, false
	}
	certs, err := certificates.ParsePEMCerts(caBytes)
	if err != nil {
		log.Info("Cannot parse PEM cert from CA secret, will create a new one", "err", err)
		return nil, false
	}
	if len(certs) == 0 {
		return nil, false
	}
	if len(certs) > 1 {
		log.Error(errors.New("more than 1 certificate in the CA, continuing with the first one"), "secret", certSecret.Name)
	}
	cert := certs[0]

	if privateKeySecret.Data == nil {
		return nil, false
	}
	privateKeyBytes, exists := privateKeySecret.Data[CAPrivateKeyFileName]
	if !exists || len(privateKeyBytes) == 0 {
		return nil, false
	}
	privateKey, err := certificates.ParsePEMPrivateKey(privateKeyBytes)
	if err != nil {
		log.Info("Cannot parse PEM private key from CA secret, will create a new one", "err", err)
		return nil, false
	}
	return certificates.NewCA(privateKey, cert), true
}

// secretsForCA returns a private key secret and a cert secret for the given CA.
func secretsForCA(ca certificates.CA, cluster types.NamespacedName) (privateKey corev1.Secret, cert corev1.Secret) {
	privateKey = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      caPrivateKeySecretName(cluster.Name),
		},
		Data: map[string][]byte{
			CAPrivateKeyFileName: certificates.EncodePEMPrivateKey(*ca.PrivateKey),
		},
	}
	cert = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      CACertSecretName(cluster.Name),
		},
		Data: map[string][]byte{
			certificates.CAFileName: certificates.EncodePEMCert(ca.Cert.Raw),
		},
	}
	return privateKey, cert
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package nodecerts

import (
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// CACommonName is the "dummy" common name used to build the CA certificate
	CACommonName = "elasticsearch-controller"
	// CaPrivateKeyFileName is the name of the private key section in the CA secret
	CaPrivateKeyFileName = "private.key"

	// CertExpirationSafetyMargin specifies how long before its expiration the CA cert should be rotated
	CertExpirationSafetyMargin = 1 * time.Hour
)

// GetOrCreateControllerCA ensures a valid CA exists for this controller, and returns it.
//
// If it does not already exist, it will be created.
// If it already exists but will expire soon or is invalid, it will be recreated.
//
// Both CA and private key are persisted as a secret in the apiserver.
// TODO: other options for persisting the secret in a more secure place.
func GetOrCreateControllerCA(clientConfig *rest.Config, newCertExpiration time.Duration) (*certificates.Ca, error) {
	// don't rely on the controller-runtime client,
	// since we don't want to cache resources here
	clientset, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return nil, err
	}

	nsn := controllerCASecretName()

	secret, err := clientset.CoreV1().Secrets(nsn.Namespace).Get(nsn.Name, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}

	if apierrors.IsNotFound(err) {
		log.Info("CA secret not found: creating a new CA", "secret", nsn.Name)
		secret, ca, err := CreateCaSecret(nsn, newCertExpiration)
		if err != nil {
			return nil, err
		}
		_, err = clientset.CoreV1().Secrets(nsn.Namespace).Create(secret)
		return ca, err
	}

	ca, canBeParsed := CaFromSecret(*secret)
	if err != nil {
		return nil, err
	}

	if !canBeParsed || shouldUpdateCACert(*ca.Cert) {
		log.Info("Setting up a new CA to replace the existing one", "secret", nsn.Name)
		secret, ca, err := CreateCaSecret(nsn, newCertExpiration)
		if err != nil {
			return nil, err
		}
		_, err = clientset.CoreV1().Secrets(nsn.Namespace).Update(secret)
		return ca, err
	}

	log.Info("Reusing the existing CA", "secret", nsn.Name)
	return ca, nil
}

// CreateCaSecret creates a new self-signed CA, and stores it in a secret with
// the given namespace and name.
func CreateCaSecret(nsn types.NamespacedName, expireIn time.Duration) (*corev1.Secret, *certificates.Ca, error) {
	privateKey, err := rsa.GenerateKey(cryptorand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	ca, err := certificates.NewSelfSignedCa(certificates.CABuilderOptions{
		CommonName: CACommonName,
		PrivateKey: privateKey,
		ExpireIn:   &expireIn,
	})
	if err != nil {
		return nil, nil, err
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: nsn.Namespace,
			Name:      nsn.Name,
		},
		Data: map[string][]byte{
			certificates.CAFileName: certificates.EncodePEMCert(ca.Cert.Raw),
			CaPrivateKeyFileName:    certificates.EncodePEMPrivateKey(*privateKey),
		},
	}, ca, nil
}

// CaFromSecret parses the given secret into a Ca.
func CaFromSecret(secret corev1.Secret) (*certificates.Ca, bool) {
	if secret.Data == nil {
		return nil, false
	}
	caBytes, exists := secret.Data[certificates.CAFileName]
	if !exists || len(caBytes) == 0 {
		return nil, false
	}
	priateKeyBytes, exists := secret.Data[CaPrivateKeyFileName]
	if !exists || len(priateKeyBytes) == 0 {
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
	// TODO: handle combined certs in certificates.Ca
	cert := certs[0]
	privateKey, err := certificates.ParsePEMPrivateKey(priateKeyBytes)
	if err != nil {
		log.Info("Cannot parse PEM private key from CA secret, will create a new ones", "err", err)
		return nil, false
	}
	return certificates.NewCa(privateKey, cert), true
}

// shouldUpdateCACert returns false if the given cert is valid,
// according to a safety time margin.
// Otherwise, it returns true, indicating the CA cert should be updated.
func shouldUpdateCACert(cert x509.Certificate) bool {
	// check that at least one of the CA certs is valid
	now := time.Now()
	if now.Before(cert.NotBefore) {
		log.Info("CA is not valid yet, will create a new one")
		return true
	}
	if now.After(cert.NotAfter.Add(-CertExpirationSafetyMargin)) {
		log.Info("CA expired or soon to expire, will create a new one", "expiration", cert.NotAfter)
		return true
	}
	return false
}

// controllerCASecretName returns the namespace and name of the
// secret containing the controller CA.
//
// Values are made of the namespace and pod this program is running in.
// If not running in kubernetes, it defaults to "default" and "elastic-operator-ca".
func controllerCASecretName() types.NamespacedName {
	// the CA secret is stored in the current namespace
	currentNamespace, err := k8s.CurrentNamespace()
	if err != nil {
		currentNamespace = "default"
		log.Info(
			"Could not guess the k8s namespace this program is running in, using default value",
			"default", currentNamespace,
			"err", err,
		)
	}
	// and its name is based on the current pod name
	currentPodName, err := k8s.CurrentPodName()
	if err != nil {
		currentPodName = "elastic-operator"
		log.Info(
			"Could not guess the k8s pod name this program is running in, using default value",
			"default", currentPodName,
			"err", err,
		)
	}

	return types.NamespacedName{
		Namespace: currentNamespace,
		Name:      currentPodName + "-ca",
	}
}

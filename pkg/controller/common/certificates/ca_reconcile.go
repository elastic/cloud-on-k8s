// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package certificates

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// CAType is a type of CA
type CAType string

const (
	// TransportCAType is the CA used for ES transport certificates
	TransportCAType CAType = "transport"
	// HTTPCAType is the CA used for HTTP certificates
	HTTPCAType CAType = "http"
)

const (
	caInternalSecretSuffix = "ca-internal"
)

// CAInternalSecretName returns the name of the internal secret containing the CA certs and keys
func CAInternalSecretName(namer name.Namer, ownerName string, caType CAType) string {
	return namer.Suffix(ownerName, string(caType), caInternalSecretSuffix)
}

// ReconcileCAForOwner ensures that a CA exists for the given owner and CAType, and returns it.
//
// The CA is persisted across operator restarts in the apiserver as a Secret for the CA certificate and private key:
// `<clusterName>-<caType>-ca-internal`
//
// The CA cert and private key are rotated if they become invalid (or soon to expire).
func ReconcileCAForOwner(
	cl k8s.Client,
	namer name.Namer,
	owner v1.Object,
	labels map[string]string,
	caType CAType,
	rotationParams RotationParams,
) (*CA, error) {

	// retrieve current CA secret
	caInternalSecret := corev1.Secret{}
	err := cl.Get(types.NamespacedName{
		Namespace: owner.GetNamespace(),
		Name:      CAInternalSecretName(namer, owner.GetName(), caType),
	}, &caInternalSecret)

	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	if apierrors.IsNotFound(err) {
		log.Info("No internal CA certificate Secret found, creating a new one", "owner_namespace", owner.GetNamespace(), "owner_name", owner.GetName(), "ca_type", caType)
		return renewCA(cl, namer, owner, labels, rotationParams.Validity, caType)
	}

	// build CA
	ca := BuildCAFromSecret(caInternalSecret)
	if ca == nil {
		log.Info("Cannot build CA from secret, creating a new one", "owner_namespace", owner.GetNamespace(), "owner_name", owner.GetName(), "ca_type", caType)
		return renewCA(cl, namer, owner, labels, rotationParams.Validity, caType)
	}

	// renew if cannot reuse
	if !CanReuseCA(ca, rotationParams.RotateBefore) {
		log.Info("Cannot reuse existing CA, creating a new one", "owner_namespace", owner.GetNamespace(), "owner_name", owner.GetName(), "ca_type", caType)
		return renewCA(cl, namer, owner, labels, rotationParams.Validity, caType)
	}

	// reuse existing CA
	return ca, nil
}

// renewCA creates and stores a new CA to replace one that might exist
func renewCA(
	client k8s.Client,
	namer name.Namer,
	owner v1.Object,
	labels map[string]string,
	expireIn time.Duration,
	caType CAType,
) (*CA, error) {
	ca, err := NewSelfSignedCA(CABuilderOptions{
		Subject: pkix.Name{
			CommonName:         owner.GetName() + "-" + string(caType),
			OrganizationalUnit: []string{owner.GetName()},
		},
		ExpireIn: &expireIn,
	})
	if err != nil {
		return nil, err
	}
	caInternalSecret := internalSecretForCA(ca, namer, owner, labels, caType)

	// create or update internal secret
	if _, err := reconciler.ReconcileSecret(client, caInternalSecret, owner); err != nil {
		return nil, err
	}

	return ca, nil
}

// CanReuseCA returns true if the given CA is valid for reuse
func CanReuseCA(ca *CA, expirationSafetyMargin time.Duration) bool {
	return PrivateMatchesPublicKey(ca.Cert.PublicKey, *ca.PrivateKey) && CertIsValid(*ca.Cert, expirationSafetyMargin)
}

// CertIsValid returns true if the given cert is valid,
// according to a safety time margin.
func CertIsValid(cert x509.Certificate, expirationSafetyMargin time.Duration) bool {
	now := time.Now()
	if now.Before(cert.NotBefore) {
		log.Info("CA cert is not valid yet", "subject", cert.Subject)
		return false
	}
	if now.After(cert.NotAfter.Add(-expirationSafetyMargin)) {
		log.Info("CA cert expired or soon to expire", "subject", cert.Subject, "expiration", cert.NotAfter)
		return false
	}
	return true
}

// internalSecretForCA returns a new internal Secret for the given CA.
func internalSecretForCA(
	ca *CA,
	namer name.Namer,
	owner v1.Object,
	labels map[string]string,
	caType CAType,
) corev1.Secret {
	return corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Namespace: owner.GetNamespace(),
			Name:      CAInternalSecretName(namer, owner.GetName(), caType),
			Labels:    labels,
		},
		Data: map[string][]byte{
			CertFileName: EncodePEMCert(ca.Cert.Raw),
			KeyFileName:  EncodePEMPrivateKey(*ca.PrivateKey),
		},
	}
}

// BuildCAFromSecret parses the given secret into a CA.
// It returns nil if the secrets could not be parsed into a CA.
func BuildCAFromSecret(caInternalSecret corev1.Secret) *CA {
	if caInternalSecret.Data == nil {
		return nil
	}
	caBytes, exists := caInternalSecret.Data[CertFileName]
	if !exists || len(caBytes) == 0 {
		return nil
	}
	certs, err := ParsePEMCerts(caBytes)
	if err != nil {
		log.Error(err, "Cannot parse PEM cert from CA secret, will create a new one", "namespace", caInternalSecret.Namespace, "secret_name", caInternalSecret.Name)
		return nil
	}
	if len(certs) == 0 {
		return nil
	}
	if len(certs) > 1 {
		log.Info(
			"More than 1 certificate in the CA secret, continuing with the first one",
			"namespace", caInternalSecret.Namespace,
			"secret_name", caInternalSecret.Name,
		)
	}
	cert := certs[0]

	privateKeyBytes, exists := caInternalSecret.Data[KeyFileName]
	if !exists || len(privateKeyBytes) == 0 {
		return nil
	}
	privateKey, err := ParsePEMPrivateKey(privateKeyBytes)
	if err != nil {
		log.Error(err, "Cannot parse PEM private key from CA secret, will create a new one", "namespace", caInternalSecret.Namespace, "secret_name", caInternalSecret.Name)
		return nil
	}
	return NewCA(privateKey, cert)
}

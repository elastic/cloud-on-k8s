// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package certificates

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/fs"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
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
	ctx context.Context,
	cl k8s.Client,
	namer name.Namer,
	owner client.Object,
	labels map[string]string,
	caType CAType,
	rotationParams RotationParams,
) (*CA, error) {
	log := ulog.FromContext(ctx)
	// retrieve current CA secret
	caInternalSecret := corev1.Secret{}
	err := cl.Get(ctx, types.NamespacedName{
		Namespace: owner.GetNamespace(),
		Name:      CAInternalSecretName(namer, owner.GetName(), caType),
	}, &caInternalSecret)

	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	if apierrors.IsNotFound(err) {
		log.Info("No internal CA certificate Secret found, creating a new one", "owner_namespace", owner.GetNamespace(), "owner_name", owner.GetName(), "ca_type", caType)
		return renewCA(ctx, cl, namer, owner, labels, rotationParams.Validity, caType)
	}

	// build CA
	ca := BuildCAFromSecret(ctx, caInternalSecret)
	if ca == nil {
		log.Info("Cannot build CA from secret, creating a new one", "owner_namespace", owner.GetNamespace(), "owner_name", owner.GetName(), "ca_type", caType)
		return renewCA(ctx, cl, namer, owner, labels, rotationParams.Validity, caType)
	}

	// renew or recreate from private key if cannot reuse
	if !CanReuseCA(ctx, ca, rotationParams.RotateBefore) {
		if ca.PrivateKey != nil && certExpiring(time.Now(), *ca.Cert, rotationParams.RotateBefore) {
			log.Info("Existing CA is expiring, creating a new one from existing private key", "owner_namespace", owner.GetNamespace(), "owner_name", owner.GetName(), "ca_type", caType)
			return renewCAFromExisting(ctx, cl, namer, owner, labels, rotationParams.Validity, caType, ca.PrivateKey)
		}
		log.Info("Cannot reuse existing CA, creating a new one", "owner_namespace", owner.GetNamespace(), "owner_name", owner.GetName(), "ca_type", caType)
		return renewCA(ctx, cl, namer, owner, labels, rotationParams.Validity, caType)
	}

	// reuse existing CA
	return ca, nil
}

// renewCAFromExisting will attempt to renew, or rather create a new CA using the existing
// private key from the existing CA, using the same options as the previous CA. There are 2
// scenarios where this will fail back to the existing behavior of creating a new CA with
// a newly created private key and those are:
// 1. The given CA is nil
// 2. The CA's private key interface type cannot be asserted to be a *rsa.PrivateKey
func renewCAFromExisting(
	ctx context.Context,
	client k8s.Client,
	namer name.Namer,
	owner client.Object,
	labels map[string]string,
	expireIn time.Duration,
	caType CAType,
	signer crypto.Signer,
) (*CA, error) {
	log := ulog.FromContext(ctx)
	privateKey, ok := signer.(*rsa.PrivateKey)
	if !ok {
		log.Error(
			errors.New("cannot cast ca.PrivateKey into *rsa.PrivateKey"),
			"Failed to cast the operator generated CA private key into a RSA private key",
			"namespace", owner.GetNamespace(),
			"name", owner.GetName(),
			"type", fmt.Sprintf("%T", signer),
		)
		return renewCA(ctx, client, namer, owner, labels, expireIn, caType)
	}

	log.Info(
		"Attempting to renew CA certificate with existing private key",
		"namespace", owner.GetNamespace(),
		"name", owner.GetName(),
	)
	return renewCAWithOptions(ctx, client, namer, owner, labels, caType, CABuilderOptions{
		Subject: pkix.Name{
			CommonName:         owner.GetName() + "-" + string(caType),
			OrganizationalUnit: []string{owner.GetName()},
		},
		ExpireIn:   &expireIn,
		PrivateKey: privateKey,
	})
}

// renewCA creates and stores a new CA to replace one that might exist using a set of default builder options.
func renewCA(
	ctx context.Context,
	client k8s.Client,
	namer name.Namer,
	owner client.Object,
	labels map[string]string,
	expireIn time.Duration,
	caType CAType,
) (*CA, error) {
	return renewCAWithOptions(ctx, client, namer, owner, labels, caType, CABuilderOptions{
		Subject: pkix.Name{
			CommonName:         owner.GetName() + "-" + string(caType),
			OrganizationalUnit: []string{owner.GetName()},
		},
		ExpireIn: &expireIn,
	})
}

// renewCAWithOptions will create and store a new CA to replace one that might exist using a set of given builder options
// instead of accepting the defaults.
func renewCAWithOptions(
	ctx context.Context,
	client k8s.Client,
	namer name.Namer,
	owner client.Object,
	labels map[string]string,
	caType CAType,
	options CABuilderOptions,
) (*CA, error) {
	ca, err := NewSelfSignedCA(options)
	if err != nil {
		return nil, err
	}
	caInternalSecret, err := internalSecretForCA(ca, namer, owner, labels, caType)
	if err != nil {
		return nil, err
	}

	// create or update internal secret
	if _, err := reconciler.ReconcileSecret(ctx, client, caInternalSecret, owner); err != nil {
		return nil, err
	}

	return ca, nil
}

// CanReuseCA returns true if the given CA is valid for reuse
func CanReuseCA(ctx context.Context, ca *CA, expirationSafetyMargin time.Duration) bool {
	return PrivateMatchesPublicKey(ctx, ca.Cert.PublicKey, ca.PrivateKey) && CertIsValid(ctx, *ca.Cert, expirationSafetyMargin)
}

// CertIsValid returns true if the given cert is valid,
// according to a safety time margin.
func CertIsValid(ctx context.Context, cert x509.Certificate, expirationSafetyMargin time.Duration) bool {
	log := ulog.FromContext(ctx)
	now := time.Now()
	if now.Before(cert.NotBefore) {
		log.Info("CA cert is not valid yet", "subject", cert.Subject)
		return false
	}
	if certExpiring(now, cert, expirationSafetyMargin) {
		log.Info("CA cert expired or soon to expire", "subject", cert.Subject, "expiration", cert.NotAfter)
		return false
	}
	return true
}

// certExpiring is a simple helper function to see if a certificate is expiring relative to the given
// time.Time, and a given safety margin.
func certExpiring(t time.Time, cert x509.Certificate, expirationSafetyMargin time.Duration) bool {
	return t.After(cert.NotAfter.Add(-expirationSafetyMargin))
}

// internalSecretForCA returns a new internal Secret for the given CA.
func internalSecretForCA(
	ca *CA,
	namer name.Namer,
	owner v1.Object,
	labels map[string]string,
	caType CAType,
) (corev1.Secret, error) {
	privateKeyData, err := EncodePEMPrivateKey(ca.PrivateKey)
	if err != nil {
		return corev1.Secret{}, err
	}
	return corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Namespace: owner.GetNamespace(),
			Name:      CAInternalSecretName(namer, owner.GetName(), caType),
			Labels:    labels,
		},
		Data: map[string][]byte{
			CertFileName: EncodePEMCert(ca.Cert.Raw),
			KeyFileName:  privateKeyData,
		},
	}, nil
}

func detectCAFileNames(path string) (string, string, error) {
	dirExists, err := fs.FileExists(path)
	if err != nil {
		return "", "", err
	}
	if !dirExists {
		return "", "", fmt.Errorf("global CA directory %s does not exist", path)
	}

	caFiles := []string{CAFileName, CAKeyFileName}
	tlsFiles := []string{CertFileName, KeyFileName}
	existsInDirectory := map[string]bool{}
	for _, f := range append(caFiles, tlsFiles...) {
		exists, err := fs.FileExists(filepath.Join(path, f))
		if err != nil {
			return "", "", err
		}
		existsInDirectory[f] = exists
	}
	switch {
	case (existsInDirectory[CertFileName] || existsInDirectory[KeyFileName]) && existsInDirectory[CAKeyFileName]:
		return "", "", fmt.Errorf("both tls.* and ca.* files exist, configuration error")
	case existsInDirectory[CAFileName] && existsInDirectory[CAKeyFileName]:
		return filepath.Join(path, CAFileName), filepath.Join(path, CAKeyFileName), nil
	case existsInDirectory[CertFileName] && existsInDirectory[KeyFileName]:
		return filepath.Join(path, CertFileName), filepath.Join(path, KeyFileName), nil
	}
	return "", "",
		fmt.Errorf(
			"no CA certificate files found in %s, expecting one of the following key pair: (%s) or (%s)",
			path,
			strings.Join(caFiles, ","),
			strings.Join(tlsFiles, ","))
}

// BuildCAFromFile reads and parses a CA and its associated private from files under path. Two naming conventions are supported:
// tls.key and tls.crt or ca.key and ca.crt for private key and certificate respectively.
func BuildCAFromFile(path string) (*CA, error) {
	certFile, privateKeyFile, err := detectCAFileNames(path)
	if err != nil {
		return nil, err
	}

	bytes, err := os.ReadFile(certFile)
	if err != nil {
		return nil, err
	}
	certs, err := ParsePEMCerts(bytes)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot parse PEM cert from %s", certFile)
	}

	if len(certs) == 0 {
		return nil, fmt.Errorf("PEM %s file does not contain any certificates", certFile)
	}

	if len(certs) > 1 {
		return nil, fmt.Errorf("more than one certificate in PEM file %s", certFile)
	}
	cert := certs[0]

	privateKeyBytes, err := os.ReadFile(privateKeyFile)
	if err != nil {
		return nil, err
	}
	privateKey, err := ParsePEMPrivateKey(privateKeyBytes)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot parse private key from PEM file %s", privateKeyFile)
	}
	return NewCA(privateKey, cert), nil
}

// BuildCAFromSecret parses the given secret into a CA.
// It returns nil if the secrets could not be parsed into a CA.
func BuildCAFromSecret(ctx context.Context, caInternalSecret corev1.Secret) *CA {
	if caInternalSecret.Data == nil {
		return nil
	}
	log := ulog.FromContext(ctx)
	caBytes, exists := caInternalSecret.Data[CertFileName]
	if !exists || len(caBytes) == 0 {
		return nil
	}
	certs, err := ParsePEMCerts(caBytes)
	if err != nil {
		log.Error(err, "cannot parse PEM cert from CA secret, will create a new one", "namespace", caInternalSecret.Namespace, "secret_name", caInternalSecret.Name)
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

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package nodecerts

import (
	"crypto/x509"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/reconciler"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// Certificate authority
//
// For each Elasticsearch cluster, we manage one CA (certificate + private key),
// which is then used to issue certificates for the cluster nodes.
// CA is persisted across operator restarts in the apiserver:
// - one secret for the CA certificate: `<cluster-name>-ca`
// - one secret for the CA private key: `<cluster-name>-ca-private-key`
//
// The CA certificate secret is safe to be shared, and can be reused by any HTTP client
// that needs to reach the Elasticsearch cluster. It can also be mounted as a volume
// in any client pod.
// The CA private key secret is reserved to the Elasticsearch controller only.
// TODO: store the private key in a more "secure" place (or wrapped in an encryption layer)
//
// CA cert and private key are rotated if they become invalid (or soon to expire).

const (
	// DefaultExpirationSafetyMargin defines how long before expiration a certificate
	// should be re-issued
	DefaultExpirationSafetyMargin = 24 * time.Hour
)

// ReconcileCAForCluster ensures that a CA exists for the given cluster, and returns it.
func ReconcileCAForCluster(
	cl k8s.Client,
	cluster v1alpha1.ElasticsearchCluster,
	scheme *runtime.Scheme,
	caCertValidity time.Duration,
	expirationSafetyMargin time.Duration,
) (*certificates.Ca, error) {
	// retrieve current CA cert
	caCert := corev1.Secret{}
	err := cl.Get(types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      CACertSecretName(cluster.Name),
	}, &caCert)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	if apierrors.IsNotFound(err) {
		log.Info("No CA certificate found, creating a new one", "cluster", cluster.Name)
		return renewCA(cl, cluster, caCertValidity, scheme)
	}

	// retrieve current CA private key
	caPrivateKey := corev1.Secret{}
	err = cl.Get(types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      caPrivateKeySecretName(cluster.Name),
	}, &caPrivateKey)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	if apierrors.IsNotFound(err) {
		log.Info("No CA private key found, creating a new one", "cluster", cluster.Name)
		return renewCA(cl, cluster, caCertValidity, scheme)
	}

	// build CA from both secrets
	ca, ok := caFromSecrets(caCert, caPrivateKey)
	if !ok {
		log.Info("Cannot build CA from secrets, creating a new one", "cluster", cluster.Name)
		return renewCA(cl, cluster, caCertValidity, scheme)
	}

	// renew if cannot reuse
	if !canReuseCa(*ca, expirationSafetyMargin) {
		log.Info("Cannot reuse existing CA, creating a new one", "cluster", cluster.Name)
		return renewCA(cl, cluster, caCertValidity, scheme)
	}

	// reuse existing CA
	log.V(1).Info("Reusing existing CA", "cluster", cluster.Name)
	return ca, nil
}

// renewCA creates and store a new CA to replace one that might exist
func renewCA(client k8s.Client, cluster v1alpha1.ElasticsearchCluster, expireIn time.Duration, scheme *runtime.Scheme) (*certificates.Ca, error) {
	ca, err := certificates.NewSelfSignedCa(certificates.CABuilderOptions{
		CommonName: cluster.Name,
		ExpireIn:   &expireIn,
	})
	if err != nil {
		return nil, err
	}

	privateKeySecret, certSecret := secretsForCa(*ca, k8s.ExtractNamespacedName(&cluster))

	// create or update private key secret
	reconciledPrivateKey := corev1.Secret{}
	if err := reconciler.ReconcileResource(reconciler.Params{
		Client:           client,
		Expected:         &privateKeySecret,
		NeedsUpdate:      func() bool { return true },
		Owner:            &cluster,
		Reconciled:       &reconciledPrivateKey,
		Scheme:           scheme,
		UpdateReconciled: func() { reconciledPrivateKey.Data = privateKeySecret.Data },
	}); err != nil {
		return nil, err
	}
	// create or update cert secret
	reconciledCert := corev1.Secret{}
	if err := reconciler.ReconcileResource(reconciler.Params{
		Client:           client,
		Expected:         &certSecret,
		NeedsUpdate:      func() bool { return true },
		Owner:            &cluster,
		Reconciled:       &reconciledCert,
		Scheme:           scheme,
		UpdateReconciled: func() { reconciledCert.Data = certSecret.Data },
	}); err != nil {
		return nil, err
	}

	return ca, nil
}

// canReuseCa returns true if the given Ca is valid for reuse
func canReuseCa(ca certificates.Ca, expirationSafetyMargin time.Duration) bool {
	return certificates.PrivateMatchesPublicKey(ca.Cert.PublicKey, *ca.PrivateKey) && certIsValid(*ca.Cert, expirationSafetyMargin)
}

// certIsValid returns true if the given cert is valid,
// according to a safety time margin.
func certIsValid(cert x509.Certificate, expirationSafetyMargin time.Duration) bool {
	now := time.Now()
	if now.Before(cert.NotBefore) {
		log.Info("CA cert is not valid yet, will create a new one")
		return false
	}
	if now.After(cert.NotAfter.Add(-expirationSafetyMargin)) {
		log.Info("CA cert expired or soon to expire, will create a new one", "expiration", cert.NotAfter)
		return false
	}
	return true
}

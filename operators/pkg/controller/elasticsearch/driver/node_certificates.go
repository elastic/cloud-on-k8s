// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"crypto/x509"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/nodecerts"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// reconcileNodeCertificates ensures that a CA exists for this cluster and node certificates are issued.
// It returns the CA certificate and its expiration date.
func reconcileNodeCertificates(
	c k8s.Client,
	scheme *runtime.Scheme,
	csrClient certificates.CSRClient,
	es v1alpha1.Elasticsearch,
	services []corev1.Service,
	trustRelationships []v1alpha1.TrustRelationship,
	caCertValidity time.Duration,
	caCertRotateBefore time.Duration,
	nodeCertValidity time.Duration,
	nodeCertRotateBefore time.Duration,
) (*x509.Certificate, time.Time, error) {
	// reconcile CA
	ca, err := nodecerts.ReconcileCAForCluster(c, es, scheme, caCertValidity, caCertRotateBefore)
	if err != nil {
		return nil, time.Time{}, err
	}
	// reconcile node certificates since we might have new pods (or existing pods that needs a refresh)
	if _, err := nodecerts.ReconcileNodeCertificateSecrets(c, ca, csrClient, es, services, trustRelationships, nodeCertValidity, nodeCertRotateBefore); err != nil {
		return ca.Cert, time.Time{}, err
	}
	return ca.Cert, ca.Cert.NotAfter, nil
}

// shouldRequeueIn computes the duration after which a reconciliation should be requeued
// in order for the CA cert to be rotated before it expires.
func shouldRequeueIn(now time.Time, certExpiration time.Time, caCertRotateBefore time.Duration) time.Duration {
	// make sure we are past the safety margin when requeueing, by making it a little bit shorter
	safetyMargin := caCertRotateBefore - 1*time.Second
	requeueTime := certExpiration.Add(-safetyMargin)
	requeueIn := requeueTime.Sub(now)
	if requeueIn < 0 {
		// requeue asap
		requeueIn = 0
	}
	return requeueIn
}

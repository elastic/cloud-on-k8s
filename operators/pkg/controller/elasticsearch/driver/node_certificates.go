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
func reconcileNodeCertificates(
	c k8s.Client,
	scheme *runtime.Scheme,
	csrClient certificates.CSRClient,
	es v1alpha1.Elasticsearch,
	services []corev1.Service,
	trustRelationships []v1alpha1.TrustRelationship,
	caCertValidity time.Duration,
	certExpirationSafetyMargin time.Duration,
) (*x509.Certificate, error) {
	// reconcile CA
	ca, err := nodecerts.ReconcileCAForCluster(c, es, scheme, caCertValidity, certExpirationSafetyMargin)
	if err != nil {
		return nil, err
	}
	// reconcile node certificates since we might have new pods (or existing pods that needs a refresh)
	if _, err := nodecerts.ReconcileNodeCertificateSecrets(c, ca, csrClient, es, services, trustRelationships); err != nil {
		return ca.Cert, err
	}
	return ca.Cert, nil
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/nodecerts"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// reconcileNodeCertificates ensures that the CA cert is pushed to the API and node certificates are issued.
func reconcileNodeCertificates(
	c k8s.Client,
	scheme *runtime.Scheme,
	ca *certificates.Ca,
	csrClient certificates.CSRClient,
	es v1alpha1.ElasticsearchCluster,
	services []corev1.Service,
	trustRelationships []v1alpha1.TrustRelationship,
) error {
	// TODO: suffix with type (-ca?) and trim
	clusterCAPublicSecretObjectKey := k8s.ExtractNamespacedName(&es)
	if err := ca.ReconcilePublicCertsSecret(c, clusterCAPublicSecretObjectKey, &es, scheme); err != nil {
		return err
	}

	// reconcile node certificates since we might have new pods (or existing pods that needs a refresh)
	if _, err := nodecerts.ReconcileNodeCertificateSecrets(c, ca, csrClient, es, services, trustRelationships); err != nil {
		return err
	}

	return nil
}

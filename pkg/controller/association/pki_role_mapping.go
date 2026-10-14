// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package association

import (
	"context"
	"crypto/x509"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/rolemapping"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

const pkiRoleMappingRequestSuffix = "pki-role-mapping"

// PKIRoleMappingRequestKey returns the name of the Elasticsearch-side PKI role mapping request Secret.
func PKIRoleMappingRequestKey(
	association commonv1.Association,
	esNamespace string,
	associationName string,
) types.NamespacedName {
	return UserKey(association, esNamespace, associationName+"-"+pkiRoleMappingRequestSuffix)
}

// reconcilePKIRoleMappingRequest writes the request Secret into the ES namespace, with the client certificate public
// key fingerprint and the roles to grant. It reports whether the ES driver has evaluated this request (see
// rolemapping.PKIEvaluationAnnotation) and, if so, the conflict that withholds its role mapping, empty when there is
// none.
func reconcilePKIRoleMappingRequest(
	ctx context.Context,
	c k8s.Client,
	association commonv1.Association,
	assocMeta metadata.Metadata,
	associationName string,
	clientCert *x509.Certificate,
	roles string,
	es esv1.Elasticsearch,
) (evaluated bool, conflict string, err error) {
	requestKey := PKIRoleMappingRequestKey(association, es.Namespace, associationName)
	fingerprint := certificates.PublicKeyFingerprint(clientCert)
	requestMeta := assocMeta.Merge(metadata.Metadata{Labels: rolemapping.RequestLabels(es)})
	expected := corev1.Secret{
		Namespace:   requestKey.Namespace,
		Name:        requestKey.Name,
		Labels:      requestMeta.Labels,
		Annotations: requestMeta.Annotations,
		Data: map[string][]byte{
			rolemapping.PublicKeyFingerprintField: []byte(fingerprint),
			rolemapping.RolesField:                []byte(roles),
			rolemapping.ResourceUIDField:          []byte(association.GetUID()),
			rolemapping.AssociationNameField:      []byte(associationName),
			rolemapping.ResourceNamespaceField:    []byte(association.GetNamespace()),
			rolemapping.ResourceNameField:         []byte(association.GetName()),
		},
	}

	// ReconcileSecret uses additive annotation merge, so the PKIEvaluationAnnotation written by the
	// ES driver is preserved even after the association controller updates the Secret data.
	reconciled, err := reconciler.ReconcileSecret(ctx, c, expected, &es)
	if err != nil {
		return false, "", err
	}
	// The annotation is missing, or describes a previous content of the request, until the ES driver evaluates it.
	evaluation, ok := rolemapping.ParseEvaluation(reconciled.Annotations[rolemapping.PKIEvaluationAnnotation])
	if !ok || !evaluation.Matches(fingerprint, roles) {
		return false, "", nil
	}
	return true, evaluation.Conflict, nil
}

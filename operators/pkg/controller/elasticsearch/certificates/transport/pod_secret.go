// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package transport

import (
	"reflect"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	// LabelCertificateType is a label key that specifies what type of certificates the secret contains
	LabelCertificateType = "certificates.elasticsearch.k8s.elastic.co/type"
	// LabelCertificateTypeTransport is the LabelCertificateType value used for transport certificates
	LabelCertificateTypeTransport = "transport"

	// LastCSRUpdateAnnotation is an annotation key to indicate the last time this secret's CSR was updated
	LastCSRUpdateAnnotation = "transport.certificates.elasticsearch.k8s.elastic.co/last-csr-update"
)

// EnsureTransportCertificateSecretExists ensures the existence and Labels of the corev1.Secret that at a later point
// in time will contain the transport certificates.
func EnsureTransportCertificateSecretExists(
	c k8s.Client,
	scheme *runtime.Scheme,
	es v1alpha1.Elasticsearch,
	pod corev1.Pod,
) (*corev1.Secret, error) {
	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: pod.Namespace,
			Name:      name.TransportCertsSecret(pod.Name),

			Labels: map[string]string{
				// a label that allows us to list secrets of this type
				LabelCertificateType: LabelCertificateTypeTransport,
				// a label referencing the related pod so we can look up the pod from this secret
				label.PodNameLabelName: pod.Name,
				// a label showing which cluster this pod belongs to
				label.ClusterNameLabelName: es.Name,
			},
		},
	}

	// reconcile the secret resource
	var reconciled corev1.Secret
	if err := reconciler.ReconcileResource(reconciler.Params{
		Client:     c,
		Scheme:     scheme,
		Owner:      &es,
		Expected:   &expected,
		Reconciled: &reconciled,
		NeedsUpdate: func() bool {
			// we only care about labels, not contents
			return reflect.DeepEqual(expected.Labels, reconciled.Labels)
		},
		UpdateReconciled: func() {
			if reconciled.Labels == nil {
				reconciled.Labels = expected.Labels
			} else {
				for k, v := range expected.Labels {
					reconciled.Labels[k] = v
				}
			}
		},
	}); err != nil {
		return nil, err
	}

	return &reconciled, nil
}

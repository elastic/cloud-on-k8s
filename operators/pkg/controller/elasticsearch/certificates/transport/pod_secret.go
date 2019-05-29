// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package transport

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// LabelAssociatedPod is a label key that indicates the resource is supposed to have a named associated pod
	LabelAssociatedPod = "transport.certificates.elasticsearch.k8s.elastic.co/associated-pod"

	// LabelSecretUsage is a label key that specifies what the secret is used for
	LabelSecretUsage = "transport.certificates.elasticsearch.k8s.elastic.co/secret-usage"
	// LabelSecretUsageTransportCertificates is the LabelSecretUsage value used for transport certificates
	LabelSecretUsageTransportCertificates = "transport-certificates"

	// LabelTransportCertificateType is a label key indicating what the transport-certificates secret is used for
	LabelTransportCertificateType = "transport.certificates.elasticsearch.k8s.elastic.co/certificate-type"
	// LabelTransportCertificateTypeElasticsearchAll is the LabelTransportCertificateType value used for Elasticsearch
	LabelTransportCertificateTypeElasticsearchAll = "elasticsearch.all"

	// LastCSRUpdateAnnotation is an annotation key to indicate the last time this secret's CSR was updated
	LastCSRUpdateAnnotation = "transport.certificates.elasticsearch.k8s.elastic.co/last-csr-update"
)

// EnsureTransportCertificateSecretExists ensures the existence of the corev1.Secret that at a later point in time will
// contain the transport certificates.
func EnsureTransportCertificateSecretExists(
	c k8s.Client,
	scheme *runtime.Scheme,
	owner metav1.Object,
	pod corev1.Pod,
	certificateType string,
	labels map[string]string,
) (*corev1.Secret, error) {
	secretRef := types.NamespacedName{
		Namespace: pod.Namespace,
		Name:      name.TransportCertsSecret(pod.Name),
	}

	var secret corev1.Secret
	if err := c.Get(secretRef, &secret); err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	} else if apierrors.IsNotFound(err) {
		secret = corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretRef.Name,
				Namespace: secretRef.Namespace,

				Labels: map[string]string{
					// store the pod that this Secret will be mounted to so we can traverse from secret -> pod
					LabelAssociatedPod:            pod.Name,
					LabelSecretUsage:              LabelSecretUsageTransportCertificates,
					LabelTransportCertificateType: certificateType,
				},
			},
		}

		// apply any provided labels
		for key, value := range labels {
			secret.Labels[key] = value
		}

		if err := controllerutil.SetControllerReference(owner, &secret, scheme); err != nil {
			return nil, err
		}

		if err := c.Create(&secret); err != nil {
			return nil, err
		}
	}

	// TODO: in the future we should consider reconciling the existing secret as well instead of leaving it untouched.
	return &secret, nil
}

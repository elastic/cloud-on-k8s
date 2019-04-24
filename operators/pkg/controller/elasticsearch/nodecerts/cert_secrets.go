// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package nodecerts

import (
	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// LabelAssociatedPod is a label key that indicates the resource is supposed to have a named associated pod
	LabelAssociatedPod = "nodecerts.elasticsearch.k8s.elastic.co/associated-pod"

	// LabelSecretUsage is a label key that specifies what the secret is used for
	LabelSecretUsage = "nodecerts.elasticsearch.k8s.elastic.co/secret-usage"
	// LabelSecretUsageNodeCertificates is the LabelSecretUsage value used for node certificates
	LabelSecretUsageNodeCertificates = "node-certificates"

	// LabelNodeCertificateType is a label key indicating what the node-certificates secret is used for
	LabelNodeCertificateType = "nodecerts.elasticsearch.k8s.elastic.co/node-certificate-type"
	// LabelNodeCertificateTypeElasticsearchAll is the LabelNodeCertificateType value used for Elasticsearch
	LabelNodeCertificateTypeElasticsearchAll = "elasticsearch.all"

	// LastCSRUpdateAnnotation is an annotation key to indicate the last time this secret's CSR was updated
	LastCSRUpdateAnnotation = "nodecerts.elasticsearch.k8s.elastic.co/last-csr-update"
)

const (
	// CertFileName is used for the Certificates inside a secret
	CertFileName = "cert.pem"
	// CSRFileName is used for the CSR inside a secret
	CSRFileName = "csr.pem"
)

func findNodeCertificateSecrets(
	c k8s.Client,
	es v1alpha1.Elasticsearch,
) ([]corev1.Secret, error) {
	var nodeCertificateSecrets corev1.SecretList

	listOptions := client.ListOptions{
		Namespace: es.Namespace,
		LabelSelector: labels.Set(map[string]string{
			label.ClusterNameLabelName: es.Name,
			LabelSecretUsage:           LabelSecretUsageNodeCertificates,
		}).AsSelector(),
	}
	if err := c.List(&listOptions, &nodeCertificateSecrets); err != nil {
		return nil, err
	}

	return nodeCertificateSecrets.Items, nil
}

// EnsureNodeCertificateSecretExists ensures the existence of the corev1.Secret that at a later point in time will
// contain the node certificates.
func EnsureNodeCertificateSecretExists(
	c k8s.Client,
	scheme *runtime.Scheme,
	owner metav1.Object,
	pod corev1.Pod,
	nodeCertificateType string,
	labels map[string]string,
) (*corev1.Secret, error) {
	secretRef := types.NamespacedName{
		Namespace: pod.Namespace,
		Name:      name.CertsSecret(pod.Name),
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
					LabelAssociatedPod:       pod.Name,
					LabelSecretUsage:         LabelSecretUsageNodeCertificates,
					LabelNodeCertificateType: nodeCertificateType,
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

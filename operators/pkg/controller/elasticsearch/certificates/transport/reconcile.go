// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package transport

import (
	"bytes"
	"reflect"
	"strings"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("transport")

// ReconcileTransportCertificatesSecrets reconciles the secret containing transport certificates for all nodes in the
// cluster.
func ReconcileTransportCertificatesSecrets(
	c k8s.Client,
	scheme *runtime.Scheme,
	ca *certificates.CA,
	es v1alpha1.Elasticsearch,
	services []corev1.Service,
	rotationParams certificates.RotationParams,
) (reconcile.Result, error) {
	log.Info("Reconciling transport certificates secrets")

	var pods corev1.PodList
	if err := c.List(&client.ListOptions{
		LabelSelector: label.NewLabelSelectorForElasticsearch(es),
		Namespace:     es.Namespace,
	}, &pods); err != nil {
		return reconcile.Result{}, err
	}

	secret, err := ensureTransportCertificatesSecretExists(c, scheme, es)
	if err != nil {
		return reconcile.Result{}, err
	}
	// defensive copy of the current secret so we can check whether we need to update later on
	currentTransportCertificatesSecret := secret.DeepCopy()

	for _, pod := range pods.Items {
		if pod.Status.PodIP == "" {
			log.Info("Skipping pod because it has no IP yet", "pod", pod.Name)
			continue
		}

		if err := ensureTransportCertificatesSecretContentsForPod(
			es, secret, pod, services, ca, rotationParams,
		); err != nil {
			return reconcile.Result{}, err
		}
	}

	// remove certificates and keys for deleted pods
	podsByName := pod.PodsByName(pods.Items)
	keysToPrune := make([]string, 0)
	for secretDataKey := range secret.Data {
		if secretDataKey == certificates.CAFileName {
			// never remove the CA file
			continue
		}

		// get the pod name from the secret key name (the first segment before the ".")
		podNameForKey := strings.SplitN(secretDataKey, ".", 2)[0]

		if _, ok := podsByName[podNameForKey]; !ok {
			// pod no longer exists, so the element is safe to delete.
			keysToPrune = append(keysToPrune, secretDataKey)
		}
	}
	if len(keysToPrune) > 0 {
		log.Info("Pruning keys from certificates secret", "keys", keysToPrune)

		for _, keyToRemove := range keysToPrune {
			delete(secret.Data, keyToRemove)
		}
	}

	caBytes := certificates.EncodePEMCert(ca.Cert.Raw)

	// compare with current trusted CA certs.
	if !bytes.Equal(caBytes, secret.Data[certificates.CAFileName]) {
		secret.Data[certificates.CAFileName] = caBytes
	}

	if !reflect.DeepEqual(secret, currentTransportCertificatesSecret) {
		if err := c.Update(secret); err != nil {
			return reconcile.Result{}, err
		}
		for _, pod := range pods.Items {
			annotation.MarkPodAsUpdated(c, pod)
		}
	}

	return reconcile.Result{}, nil
}

// ensureTransportCertificatesSecretExists ensures the existence and Labels of the Secret that at a later point
// in time will contain the transport certificates.
func ensureTransportCertificatesSecretExists(
	c k8s.Client,
	scheme *runtime.Scheme,
	es v1alpha1.Elasticsearch,
) (*corev1.Secret, error) {
	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      name.TransportCertificatesSecret(es.Name),

			Labels: map[string]string{
				// a label showing which es these certificates belongs to
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
			// we only care about labels, not contents at this point, and we can allow additional labels
			if reconciled.Labels == nil {
				return true
			}

			for k, v := range expected.Labels {
				if rv, ok := reconciled.Labels[k]; !ok || rv != v {
					return true
				}
			}
			return false
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

	// a placeholder secret may have nil entries, create them if needed
	if reconciled.Data == nil {
		reconciled.Data = make(map[string][]byte)
	}

	return &reconciled, nil
}

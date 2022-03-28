// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package transport

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/pkg/utils/maps"
)

var log = ulog.Log.WithName("transport")

// ReconcileTransportCertificatesSecrets reconciles the secret containing transport certificates for all nodes in the
// cluster.
// Secrets which are not used anymore are deleted as part of the downscale process.
func ReconcileTransportCertificatesSecrets(
	c k8s.Client,
	ca *certificates.CA,
	es esv1.Elasticsearch,
	rotationParams certificates.RotationParams,
) *reconciler.Results {
	results := &reconciler.Results{}

	// We must create transport certificates for the following StatefulSets:
	// - the ones that still exist, even if they have been removed from the Spec
	// - the ones that do not exist yet, but will be created in a later step of the reconciliation
	actualStatefulSets, err := sset.RetrieveActualStatefulSets(c, k8s.ExtractNamespacedName(&es))
	if err != nil {
		return results.WithError(err)
	}
	ssets := actualStatefulSets.Names()
	for _, nodeSet := range es.Spec.NodeSets {
		ssets.Add(esv1.StatefulSet(es.Name, nodeSet.Name))
	}

	for ssetName := range ssets {
		if err := reconcileNodeSetTransportCertificatesSecrets(c, ca, es, ssetName, rotationParams); err != nil {
			results.WithError(err)
		}
	}
	return results
}

// DeleteStatefulSetTransportCertificate removes the Secret which contains the transport certificates of a given Statefulset.
func DeleteStatefulSetTransportCertificate(client k8s.Client, namespace string, ssetName string) error {
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      esv1.StatefulSetTransportCertificatesSecret(ssetName),
		},
	}
	return client.Delete(context.Background(), &secret)
}

// DeleteLegacyTransportCertificate ensures that the former Secret which used to contain the transport certificates is deleted.
func DeleteLegacyTransportCertificate(client k8s.Client, es esv1.Elasticsearch) error {
	nsn := types.NamespacedName{Namespace: es.Namespace, Name: esv1.LegacyTransportCertsSecretSuffix(es.Name)}
	return k8s.DeleteSecretIfExists(client, nsn)
}

// reconcileNodeSetTransportCertificatesSecrets reconciles the secret which contains the transport certificates for
// a given StatefulSet.
func reconcileNodeSetTransportCertificatesSecrets(
	c k8s.Client,
	ca *certificates.CA,
	es esv1.Elasticsearch,
	ssetName string,
	rotationParams certificates.RotationParams,
) error {
	results := &reconciler.Results{}
	// List all the existing Pods in the nodeSet
	var pods corev1.PodList
	matchLabels := label.NewLabelSelectorForStatefulSetName(es.Name, ssetName)
	ns := client.InNamespace(es.Namespace)
	if err := c.List(context.Background(), &pods, matchLabels, ns); err != nil {
		return errors.WithStack(err)
	}

	secret, err := ensureTransportCertificatesSecretExists(c, es, ssetName)
	if err != nil {
		return err
	}
	// defensive copy of the current secret so we can check whether we need to update later on
	currentTransportCertificatesSecret := secret.DeepCopy()
	for _, pod := range pods.Items {
		if pod.Status.PodIP == "" {
			log.Info("Skipping pod because it has no IP yet", "namespace", pod.Namespace, "pod_name", pod.Name)
			continue
		}

		if err := ensureTransportCertificatesSecretContentsForPod(
			es, secret, pod, ca, rotationParams,
		); err != nil {
			return err
		}
		certCommonName := buildCertificateCommonName(pod, es)
		cert := extractTransportCert(*secret, pod, certCommonName)
		if cert == nil {
			return errors.New("no certificate found for pod")
		}
		// handle cert expiry via requeue
		results.WithResult(reconcile.Result{
			RequeueAfter: certificates.ShouldRotateIn(time.Now(), cert.NotAfter, rotationParams.RotateBefore),
		})
	}

	// remove certificates and keys for deleted pods
	podsByName := k8s.PodsByName(pods.Items)
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
		log.Info("Pruning keys from certificates secret", "namespace", es.Namespace, "secret_name", secret.Name, "keys", keysToPrune)

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
		if err := c.Update(context.Background(), secret); err != nil {
			return err
		}
		for _, pod := range pods.Items {
			annotation.MarkPodAsUpdated(c, pod)
		}
	}

	return nil
}

// ensureTransportCertificatesSecretExists ensures the existence and labels of the Secret that at a later point
// in time will contain the transport certificates for a nodeSet.
func ensureTransportCertificatesSecretExists(
	c k8s.Client,
	es esv1.Elasticsearch,
	ssetName string,
) (*corev1.Secret, error) {
	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      esv1.StatefulSetTransportCertificatesSecret(ssetName),
			Labels: map[string]string{
				// a label showing which es these certificates belongs to
				label.ClusterNameLabelName: es.Name,
				// label indicating to which StatefulSet these certificates belong
				label.StatefulSetNameLabelName: ssetName,
			},
		},
	}
	// reconcile the secret resource:
	// - create it if it doesn't exist
	// - update labels & annotations if they don't match
	// - do not touch the existing data as it probably already contains certificates - it will be reconciled later on
	var reconciled corev1.Secret
	if err := reconciler.ReconcileResource(reconciler.Params{
		Client:     c,
		Owner:      &es,
		Expected:   &expected,
		Reconciled: &reconciled,
		NeedsUpdate: func() bool {
			return !maps.IsSubset(expected.Labels, reconciled.Labels) ||
				!maps.IsSubset(expected.Annotations, reconciled.Annotations)
		},
		UpdateReconciled: func() {
			reconciled.Labels = maps.Merge(reconciled.Labels, expected.Labels)
			reconciled.Annotations = maps.Merge(reconciled.Annotations, expected.Annotations)
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

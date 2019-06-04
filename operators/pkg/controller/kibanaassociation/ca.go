// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibanaassociation

import (
	"reflect"

	kbtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/certificates/http"
	kblabel "github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ElasticsearchCASecretSuffix is used as suffix for CAPublicCertSecretName
const ElasticsearchCASecretSuffix = "kb-es-ca"

// CAPublicCertSecretName returns the name of the secret holding Elasticsearch CA for this Kibana deployment
func CACertSecretName(kibanaName string) string {
	return kibanaName + "-" + ElasticsearchCASecretSuffix
}

// reconcileCASecret ensures a secret exists in Kibana namespace, containing the Elasticsearch CA public cert.
// The CA secret content is copied over from ES CA secret into a dedicated secret for Kibana.
func (r *ReconcileAssociation) reconcileCASecret(kibana kbtype.Kibana, es types.NamespacedName) (string, error) {
	kibanaKey := k8s.ExtractNamespacedName(&kibana)
	publicCertsSecretKey := http.PublicCertsSecretRef(es)

	// watch ES CA secret to reconcile on any change
	if err := r.watches.Secrets.AddHandler(watches.NamedWatch{
		Name:    esCAWatchName(kibanaKey),
		Watched: publicCertsSecretKey,
		Watcher: kibanaKey,
	}); err != nil {
		return "", err
	}

	// retrieve the CA from ES namespace
	var publicCACertSecret corev1.Secret
	if err := r.Get(publicCertsSecretKey, &publicCACertSecret); err != nil {
		if apierrors.IsNotFound(err) {
			return "", nil // probably not created yet, we'll be notified to reconcile later
		}
		return "", err
	}

	// CA data should be copied over a secret in Kibana namespace
	labels := kblabel.NewLabels(kibana.Name)
	labels[AssociationLabelName] = kibana.Name
	expectedSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: kibana.Namespace,
			Name:      CACertSecretName(kibana.Name),
			Labels:    labels,
		},
		Data: publicCACertSecret.Data,
	}
	var reconciledSecret corev1.Secret
	if err := reconciler.ReconcileResource(reconciler.Params{
		Client:     r.Client,
		Scheme:     r.scheme,
		Owner:      &kibana,
		Expected:   &expectedSecret,
		Reconciled: &reconciledSecret,
		NeedsUpdate: func() bool {
			return !reflect.DeepEqual(expectedSecret.Data, reconciledSecret.Data)
		},
		UpdateReconciled: func() {
			reconciledSecret.Data = expectedSecret.Data
		},
	}); err != nil {
		return "", err
	}

	return expectedSecret.Name, nil
}

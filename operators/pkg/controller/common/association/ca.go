// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	"reflect"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates/http"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	esname "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// ElasticsearchCACertSecretName returns the name of the secret holding Elasticsearch CA for this Kibana deployment
func ElasticsearchCACertSecretName(associated v1alpha1.Associated, suffix string) string {
	return associated.GetName() + "-" + suffix
}

// ReconcileCASecret keeps in sync a copy of the Elasticsearch CA.
// It is the responsibility of the controller to set watches on the ES CA.
func ReconcileCASecret(
	client k8s.Client,
	scheme *runtime.Scheme,
	associated v1alpha1.Associated,
	es types.NamespacedName,
	labels map[string]string,
	suffix string,
) (string, error) {
	publicESHTTPCertificatesNSN := http.PublicCertsSecretRef(esname.ESNamer, es)

	// retrieve the HTTP certificates from ES namespace
	var publicESHTTPCertificatesSecret corev1.Secret
	if err := client.Get(publicESHTTPCertificatesNSN, &publicESHTTPCertificatesSecret); err != nil {
		if errors.IsNotFound(err) {
			return "", nil // probably not created yet, we'll be notified to reconcile later
		}
		return "", err
	}

	// Certificate data should be copied over a secret in the Kibana or APM namespace
	expectedSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: associated.GetNamespace(),
			Name:      ElasticsearchCACertSecretName(associated, suffix),
			Labels:    labels,
		},
		Data: publicESHTTPCertificatesSecret.Data,
	}
	var reconciledSecret corev1.Secret
	if err := reconciler.ReconcileResource(reconciler.Params{
		Client:     client,
		Scheme:     scheme,
		Owner:      associated,
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

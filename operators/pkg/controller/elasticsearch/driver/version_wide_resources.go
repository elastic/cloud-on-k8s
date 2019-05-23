// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/configmap"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// VersionWideResources are resources that are tied to a version, but no specific pod within that version
type VersionWideResources struct {
	// ClusterSecrets contains possible user-defined secret files we want to have access to in the containers.
	ClusterSecrets corev1.Secret
	// GenericUnecryptedConfigurationFiles contains non-secret files Pods with this version should have access to.
	GenericUnecryptedConfigurationFiles corev1.ConfigMap
}

func reconcileVersionWideResources(
	c k8s.Client,
	scheme *runtime.Scheme,
	es v1alpha1.Elasticsearch,
) (*VersionWideResources, error) {
	expectedConfigMap := configmap.NewConfigMapWithData(k8s.ExtractNamespacedName(&es), settings.DefaultConfigMapData)
	err := configmap.ReconcileConfigMap(c, scheme, es, expectedConfigMap)
	if err != nil {
		return nil, err
	}

	// TODO: this may not exactly fit the bill of being specific to a version
	expectedClusterSecretsSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      name.ClusterSecretsSecret(es.Name),
		},
		Data: map[string][]byte{},
	}

	var reconciledClusterSecretsSecret corev1.Secret

	if err := reconciler.ReconcileResource(reconciler.Params{
		Client:     c,
		Scheme:     scheme,
		Owner:      &es,
		Expected:   &expectedClusterSecretsSecret,
		Reconciled: &reconciledClusterSecretsSecret,
		NeedsUpdate: func() bool {
			// .Data might be nil in the secret, so make sure to initialize it
			if reconciledClusterSecretsSecret.Data == nil {
				reconciledClusterSecretsSecret.Data = make(map[string][]byte, 0)
			}

			// TODO: compare items that we may want to reconcile here

			return false
		},
		UpdateReconciled: func() {
			// TODO: add items to reconcile here
		},
		PostUpdate: func() {
			annotation.MarkPodsAsUpdated(c,
				client.ListOptions{
					Namespace:     es.Namespace,
					LabelSelector: label.NewLabelSelectorForElasticsearch(es),
				})
		},
	}); err != nil {
		return nil, err
	}

	return &VersionWideResources{
		GenericUnecryptedConfigurationFiles: expectedConfigMap,
		ClusterSecrets:                      reconciledClusterSecretsSecret,
	}, nil
}

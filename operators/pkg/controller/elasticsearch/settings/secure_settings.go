// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

import (
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/reconciler"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/watches"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
)

// ReconcileSecureSettings ensures a secret containing secure settings exists for the cluster.
//
// Secure settings are provided by the user in the Elasticsearch Spec through a secret reference.
// In turn, we manage a per-cluster secret containing the same content as the user-provided secret.
// This managed secret is mounted into each pod of the cluster.
// We watch the user-provided secret, in order to copy over any change done by the user to our managed secret.
func ReconcileSecureSettings(
	c k8s.Client,
	scheme *runtime.Scheme,
	watches watches.DynamicWatches,
	es v1alpha1.Elasticsearch,
) error {
	// watch the user-provided secret to reconcile on any change
	userSecretRef := es.Spec.SecureSettings
	err := watchUserSecret(watches, userSecretRef, k8s.ExtractNamespacedName(&es))
	if err != nil {
		return err
	}

	// retrieve the secret referenced by the user
	userSecret := &corev1.Secret{}
	if userSecretRef != nil {
		userSecret, err = retrieveUserSecret(c, *userSecretRef, es.Namespace)
		if err != nil {
			return err
		}
	}

	// reconcile our managed secret with the user-provided secret content
	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.SecureSettingsSecret(es.Name),
			Namespace: es.Namespace,
		},
		Data: userSecret.Data,
	}
	reconciled := corev1.Secret{}
	return reconciler.ReconcileResource(reconciler.Params{
		Client:     c,
		Scheme:     scheme,
		Owner:      &es,
		Expected:   &expected,
		Reconciled: &reconciled,
		NeedsUpdate: func() bool {
			return !reflect.DeepEqual(expected.Data, reconciled.Data)
		},
		UpdateReconciled: func() {
			reconciled.Data = expected.Data
		},
	})
}

func retrieveUserSecret(c k8s.Client, ref corev1.SecretReference, defaultNamespace string) (*corev1.Secret, error) {
	// if the namespace is not set, default to the Elasticsearch namespace
	namespace := ref.Namespace
	if namespace == "" {
		namespace = defaultNamespace
	}
	userSecret := corev1.Secret{}
	err := c.Get(types.NamespacedName{Namespace: namespace, Name: ref.Name}, &userSecret)
	if err != nil && apierrors.IsNotFound(err) {
		log.Info("Secure settings secret not found, ignoring", "ref", ref)
	} else if err != nil {
		return nil, err
	}
	return &userSecret, nil
}

// userSecretWatchName returns the watch name according to the cluster name.
// It is unique per cluster.
func userSecretWatchName(cluster types.NamespacedName) string {
	return fmt.Sprintf("%s-%s-secure-settings", cluster.Namespace, cluster.Name)
}

// watchUserSecret registers a watch for the given user secret.
//
// Only one watch per cluster is registered:
// - if it already exists with a different secret, it is replaced to watch the new secret.
// - if the given user secret is nil, the watch is removed.
func watchUserSecret(watched watches.DynamicWatches, userSecret *corev1.SecretReference, cluster types.NamespacedName) error {
	watchName := userSecretWatchName(cluster)
	if userSecret == nil {
		watched.Secrets.RemoveHandlerForKey(watchName)
		return nil
	}
	return watched.Secrets.AddHandler(watches.NamedWatch{
		Name: watchName,
		Watched: types.NamespacedName{
			Namespace: userSecret.Namespace,
			Name:      userSecret.Name,
		},
		Watcher: cluster,
	})
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package snapshot

import (
	"fmt"
	"reflect"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/finalizer"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/reconciler"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/watches"
	esclient "github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/keystore"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/services"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/pkg/errors"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// ExternalSecretFinalizer designates a finalizer to clean up owner references in secrets not controlled by this operator.
	ExternalSecretFinalizer = "external-secret.elasticsearch.k8s.elastic.co"
)

func reconcileUserCreatedSecret(
	c k8s.Client,
	owner v1alpha1.ElasticsearchCluster,
	repoConfig *v1alpha1.SnapshotRepository,
	watches watches.DynamicWatches,
) (corev1.Secret, error) {
	managedSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      keystore.ManagedSecretName,
			Namespace: owner.Namespace,
		},
		Data: map[string][]byte{},
	}

	err := manageDynamicWatch(watches, repoConfig, k8s.ExtractNamespacedName(&owner))
	if err != nil {
		return managedSecret, err
	}

	if repoConfig == nil {
		return managedSecret, nil
	}

	secretRef := repoConfig.Settings.Credentials
	userCreatedSecret := corev1.Secret{}
	key := types.NamespacedName{Namespace: secretRef.Namespace, Name: secretRef.Name}
	if err := c.Get(key, &userCreatedSecret); err != nil {
		return managedSecret, errors.Wrap(err, "configured snapshot secret could not be retrieved")
	}

	err = ValidateSnapshotCredentials(repoConfig.Type, userCreatedSecret.Data)
	if err != nil {
		return managedSecret, err
	}

	for _, v := range userCreatedSecret.Data {
		// TODO multiple credentials?
		managedSecret.Data[RepositoryCredentialsKey(repoConfig)] = v
	}
	return managedSecret, nil
}

// ReconcileSnapshotCredentials checks the snapshot repository config for user provided secrets, validates
// snapshot repository configuration and transforms it into a managed secret to initialise
// an Elasticsearch keystore. It currently relies on a secret reference pointing to a secret
// created by the user containing valid snapshot repository credentials for the specified
// repository provider.
func ReconcileSnapshotCredentials(
	c k8s.Client,
	s *runtime.Scheme,
	owner v1alpha1.ElasticsearchCluster,
	repoConfig *v1alpha1.SnapshotRepository,
	watched watches.DynamicWatches,
) (corev1.Secret, error) {
	managedSecret, err := reconcileUserCreatedSecret(c, owner, repoConfig, watched)
	if err != nil {
		return managedSecret, err
	}

	reconciled := corev1.Secret{}
	err = reconciler.ReconcileResource(reconciler.Params{
		Client:     c,
		Scheme:     s,
		Owner:      &owner,
		Expected:   &managedSecret,
		Reconciled: &reconciled,
		NeedsUpdate: func() bool {
			return !reflect.DeepEqual(managedSecret.Data, reconciled.Data)
		},
		UpdateReconciled: func() {
			reconciled.Data = managedSecret.Data
		},
	})
	return managedSecret, err
}

func watchKey(owner types.NamespacedName) string {
	return fmt.Sprintf("%s-%s-snapshot-secret", owner.Namespace, owner.Name)
}

// Finalizer removes any dynamic watches on external user created snapshot secrets.
func Finalizer(owner types.NamespacedName, watched watches.DynamicWatches) finalizer.Finalizer {
	return finalizer.Finalizer{
		Name: ExternalSecretFinalizer,
		Execute: func() error {
			watched.Secrets.RemoveHandlerForKey(watchKey(owner))
			return nil
		},
	}
}

// manageDynamicWatch sets up a dynamic watch to keep track of changes in user created secrets linked to this ES cluster.
func manageDynamicWatch(watched watches.DynamicWatches, repoConfig *v1alpha1.SnapshotRepository, owner types.NamespacedName) error {
	if repoConfig == nil {
		watched.Secrets.RemoveHandlerForKey(watchKey(owner))
		return nil
	}

	return watched.Secrets.AddHandler(watches.NamedWatch{
		Name: watchKey(owner),
		Watched: types.NamespacedName{
			Namespace: repoConfig.Settings.Credentials.Namespace,
			Name:      repoConfig.Settings.Credentials.Name,
		},
		Watcher: owner,
	})
}

// ReconcileSnapshotterCronJob checks for an existing cron job and updates it based on the current config
func ReconcileSnapshotterCronJob(
	c k8s.Client,
	scheme *runtime.Scheme,
	es v1alpha1.ElasticsearchCluster,
	user esclient.User,
	operatorImage string,
) error {
	params := CronJobParams{
		Parent:           types.NamespacedName{Namespace: es.Namespace, Name: es.Name},
		Elasticsearch:    es,
		SnapshotterImage: operatorImage,
		User:             user,
		EsURL:            services.PublicServiceURL(es),
	}
	expected := NewCronJob(params)
	if err := controllerutil.SetControllerReference(&es, expected, scheme); err != nil {
		return err
	}

	found := &batchv1beta1.CronJob{}
	err := c.Get(k8s.ExtractNamespacedName(expected), found)
	if err == nil && es.Spec.SnapshotRepository == nil {
		log.Info("Deleting cronjob", "namespace", expected.Namespace, "name", expected.Name)
		return c.Delete(found)
	}
	if err != nil && apierrors.IsNotFound(err) {
		if es.Spec.SnapshotRepository == nil {
			return nil // we are done
		}

		log.Info("Creating cronjob", "namespace", expected.Namespace, "name", expected.Name)
		err = c.Create(expected)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	// TODO proper comparison
	if !reflect.DeepEqual(expected.Spec, found.Spec) {
		log.Info("Updating cronjob", "namespace", expected.Namespace, "name", expected.Name)
		err := c.Update(expected)
		if err != nil {
			return err
		}
	}
	return nil

}

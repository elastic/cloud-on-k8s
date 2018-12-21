package snapshot

import (
	"context"
	"reflect"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/reconciler"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	esclient "github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/keystore"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/services"
	"github.com/elastic/stack-operators/stack-operator/pkg/utils/k8s"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// SnapshotterImageFlag is the name of the flag/env-var containing the docker image of the snapshotter application.
	SnapshotterImageFlag = "snapshotter_image"
)

func reconcileUserCreatedSecret(c client.Client, owner v1alpha1.ElasticsearchCluster, repoConfig *v1alpha1.SnapshotRepository) (corev1.Secret, error) {
	managedSecret := corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      keystore.ManagedSecretName,
			Namespace: owner.Namespace,
		},
		Data: map[string][]byte{},
	}

	if repoConfig == nil {
		return managedSecret, nil
	}

	secretRef := repoConfig.Settings.Credentials
	userCreatedSecret := corev1.Secret{}
	key := types.NamespacedName{Namespace: secretRef.Namespace, Name: secretRef.Name}
	if err := c.Get(context.TODO(), key, &userCreatedSecret); err != nil {
		return managedSecret, errors.Wrap(err, "configured snapshot secret could not be retrieved")
	}

	err := ValidateSnapshotCredentials(repoConfig.Type, userCreatedSecret.Data)
	if err != nil {
		return managedSecret, err
	}

	err = ensureOwnerReference(c, userCreatedSecret, &owner)
	if err != nil {
		return managedSecret, err
	}
	for _, v := range userCreatedSecret.Data {
		// TODO multiple credentials?
		managedSecret.Data[RepositoryCredentialsKey(repoConfig)] = v
	}
	return managedSecret, nil
}

// ReconcileSnapshotCredentials checks the snapshot repository config for user provided, validates
// snapshot repository configuration and transforms it into a keystore.Config to initialise
// an Elasticsearch keystore. It currently relies on a secret reference pointing to a secret
// created by the user containing valid snapshot repository credentials for the specified
// repository provider.
func ReconcileSnapshotCredentials(
	c client.Client,
	s *runtime.Scheme,
	owner v1alpha1.ElasticsearchCluster,
	repoConfig *v1alpha1.SnapshotRepository,
) (corev1.Secret, error) {
	managedSecret, err := reconcileUserCreatedSecret(c, owner, repoConfig)
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

func ensureOwnerReference(c client.Client, secret corev1.Secret, owner runtime.Object) error {
	metaObj, err := meta.Accessor(owner)
	if err != nil {
		return err
	}
	gvk := owner.GetObjectKind().GroupVersionKind()
	blockOwnerDeletion := false
	isController := false
	ownerRef := v1.OwnerReference{
		APIVersion:         gvk.GroupVersion().String(),
		Kind:               gvk.Kind,
		Name:               metaObj.GetName(),
		UID:                metaObj.GetUID(),
		BlockOwnerDeletion: &blockOwnerDeletion,
		Controller:         &isController,
	}
	existing := secret.GetOwnerReferences()
	for _, r := range existing {
		if reflect.DeepEqual(r, ownerRef) {
			return nil
		}
	}

	existing = append(existing, ownerRef)
	secret.SetOwnerReferences(existing)
	return c.Update(context.TODO(), &secret)
}

// ReconcileSnapshotterCronJob checks for an existing cron job and updates it based on the current config
func ReconcileSnapshotterCronJob(
	c client.Client,
	scheme *runtime.Scheme,
	es v1alpha1.ElasticsearchCluster,
	user esclient.User,
) error {
	params := CronJobParams{
		Parent:           types.NamespacedName{Namespace: es.Namespace, Name: es.Name},
		Elasticsearch:    es,
		SnapshotterImage: viper.GetString(SnapshotterImageFlag),
		User:             user,
		EsURL:            services.PublicServiceURL(es),
	}
	expected := NewCronJob(params)
	if err := controllerutil.SetControllerReference(&es, expected, scheme); err != nil {
		return err
	}

	found := &batchv1beta1.CronJob{}
	err := c.Get(context.TODO(), k8s.ExtractNamespacedName(expected.ObjectMeta), found)
	if err == nil && es.Spec.SnapshotRepository == nil {
		log.Info("Deleting cronjob", "namespace", expected.Namespace, "name", expected.Name)
		return c.Delete(context.TODO(), found)
	}
	if err != nil && apierrors.IsNotFound(err) {
		if es.Spec.SnapshotRepository == nil {
			return nil // we are done
		}

		log.Info("Creating cronjob", "namespace", expected.Namespace, "name", expected.Name)
		err = c.Create(context.TODO(), expected)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	// TODO proper comparison
	if !reflect.DeepEqual(expected.Spec, found.Spec) {
		log.Info("Updating cronjob", "namespace", expected.Namespace, "name", expected.Name)
		err := c.Update(context.TODO(), expected)
		if err != nil {
			return err
		}
	}
	return nil

}

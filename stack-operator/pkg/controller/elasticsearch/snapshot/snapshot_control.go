package snapshot

import (
	"context"
	"fmt"
	"reflect"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/finalizer"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/operator"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/reconciler"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/watches"
	esclient "github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/keystore"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/services"
	"github.com/elastic/stack-operators/stack-operator/pkg/utils/k8s"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// ExternalSecretFinalizer designates a finalizer to clean up owner references in secrets not controlled by this operator.
	ExternalSecretFinalizer = "external-secret.elasticsearch.k8s.elastic.co"
)

func reconcileUserCreatedSecret(c client.Client, owner v1alpha1.ElasticsearchCluster, repoConfig *v1alpha1.SnapshotRepository) (corev1.Secret, error) {
	managedSecret := corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      keystore.ManagedSecretName,
			Namespace: owner.Namespace,
		},
		Data: map[string][]byte{},
	}

	err := manageDynamicWatch(c, repoConfig, k8s.ExtractNamespacedName(owner.ObjectMeta))
	if err != nil {
		return managedSecret, err
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

func watchLabel(owner types.NamespacedName) string {
	return fmt.Sprintf("%s-%s-snapshot-secret", owner.Namespace, owner.Name)
}

func Finalizer(owner types.NamespacedName) finalizer.Finalizer {
	return finalizer.Finalizer{
		Name: ExternalSecretFinalizer,
		Execute: func() error {
			watches.SecretWatch.RemoveWatchForKey(watchLabel(owner))
			return nil
		},
	}
}

// manageDynamicWatch sets up a dynamic watch to keep track of changes in user created secrets linked to this ES cluster.
func manageDynamicWatch(c client.Client, repoConfig *v1alpha1.SnapshotRepository, owner types.NamespacedName) error {
	if repoConfig == nil {
		watches.SecretWatch.RemoveWatchForKey(watchLabel(owner))
		return nil
	}

	return watches.SecretWatch.AddWatch(watches.NamedWatch{
		Name: watchLabel(owner),
		Watched: types.NamespacedName{
			Namespace: repoConfig.Settings.Credentials.Namespace,
			Name:      repoConfig.Settings.Credentials.Name,
		},
		Watcher: owner,
	})
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
		SnapshotterImage: viper.GetString(operator.ImageFlag),
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

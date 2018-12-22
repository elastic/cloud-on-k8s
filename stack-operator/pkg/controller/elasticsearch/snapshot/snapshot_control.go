package snapshot

import (
	"context"
	"reflect"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/operator"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/reconciler"
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
	"k8s.io/apimachinery/pkg/apis/meta/v1"
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

	err = manageOwnerReference(c, userCreatedSecret, owner)
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

// manageOwnerReference set an owner reference into the user created secret to facilitate watching for changes
// it also sets up a finalizer to remove the owner reference on cluster deletion
func manageOwnerReference(c client.Client, secret corev1.Secret, owner v1alpha1.ElasticsearchCluster) error {
	gvk := owner.GetObjectKind().GroupVersionKind()
	blockOwnerDeletion := false
	isController := false
	ownerRef := v1.OwnerReference{
		APIVersion:         gvk.GroupVersion().String(),
		Kind:               gvk.Kind,
		Name:               owner.GetName(),
		UID:                owner.GetUID(),
		BlockOwnerDeletion: &blockOwnerDeletion,
		Controller:         &isController,
	}

	if owner.DeletionTimestamp.IsZero() {

		existing := secret.GetOwnerReferences()
		for _, r := range existing {
			if reflect.DeepEqual(r, ownerRef) {
				return nil
			}
		}
		existing = append(existing, ownerRef)
		secret.SetOwnerReferences(existing)
		log.Info("Adding owner", "secret", secret.Name, "elasticsearch", owner.Name)
		if err := c.Update(context.Background(), &secret); err != nil {
			return err
		}
		if !common.StringInSlice(ExternalSecretFinalizer, owner.Finalizers) {
			log.Info("Adding finalizer", "finalizer", ExternalSecretFinalizer)
			owner.Finalizers = append(owner.Finalizers, ExternalSecretFinalizer)
			return c.Update(context.Background(), &owner)
		}
		return nil
	}

	if common.StringInSlice(ExternalSecretFinalizer, owner.Finalizers) {
		filtered := secret.GetOwnerReferences()[:0]
		for _, r := range secret.GetOwnerReferences() {
			if !reflect.DeepEqual(r, ownerRef) {
				filtered = append(filtered, r)
			}
		}
		// TODO if the user removes the secret from the cluster, owner ref will remain in place
		// labeling as an alternative does not really work as it would prevent reuse of the secret in multiple
		// clusters
		secret.SetOwnerReferences(filtered)
		log.Info("Removing owner", "secret", secret.Name, "elasticsearch", owner.Name)
		if err := c.Update(context.Background(), &secret); err != nil {
			return err
		}

		owner.Finalizers = common.RemoveStringInSlice(ExternalSecretFinalizer, owner.Finalizers)
		log.Info("Removing finalizer", "finalizer", ExternalSecretFinalizer)
		return c.Update(context.Background(), &owner)
	}
	return nil
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

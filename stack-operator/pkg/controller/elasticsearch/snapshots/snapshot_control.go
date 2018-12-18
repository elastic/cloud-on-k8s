package snapshots

import (
	"context"
	"path"
	"reflect"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	esclient "github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/keystore"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/services"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"
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

// ReconcileSnapshotCredentials checks the snapshot repository config for user provided, validates
// snapshot repository configuration and transforms it into a keystore.Config to initialise
// an Elasticsearch keystore. It currently relies on a secret reference pointing to a secret
// created by the user containing valid snapshot repository credentials for the specified
// repository provider.
func ReconcileSnapshotCredentials(c client.Client, repoConfig *v1alpha1.SnapshotRepository) (keystore.Config, error) {
	var result keystore.Config
	if repoConfig == nil {
		return result, nil
	}

	secretRef := repoConfig.Settings.Credentials
	userCreatedSecret := corev1.Secret{}
	key := types.NamespacedName{Namespace: secretRef.Namespace, Name: secretRef.Name}
	if err := c.Get(context.TODO(), key, &userCreatedSecret); err != nil {
		return result, errors.Wrap(err, "configured snapshot secret could not be retrieved")
	}

	err := ValidateSnapshotCredentials(repoConfig.Type, userCreatedSecret.Data)
	if err != nil {
		return result, err
	}

	settings := make([]keystore.Setting, 0, len(userCreatedSecret.Data))
	for k := range userCreatedSecret.Data {
		settings = append(
			settings,
			keystore.Setting{
				Key:           RepositoryCredentialsKey(repoConfig),
				ValueFilePath: path.Join(support.KeystoreSecretMountPath, k),
			})
	}
	result.KeystoreSettings = settings
	result.KeystoreSecretRef = secretRef
	return result, nil
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

package stack

import (
	"context"
	"path"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch"
	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/keystore"
	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/snapshots"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ReconcileSnapshotCredentials checks the snapshot repository config for user provided, valid
// snapshot repository configuration and transforms them into a keystore.Config to initialise
// an Elasticsearch keystore. It currently relies on a secret reference pointing to a secret
// created by the user containing valid snapshot repository credentials for the specified
// repository provider.
func (r *ReconcileStack) ReconcileSnapshotCredentials(repoConfig deploymentsv1alpha1.SnapshotRepository) (keystore.Config, error) {

	var result keystore.Config
	empty := corev1.SecretReference{}
	userCreatedSecret := corev1.Secret{}
	if repoConfig.Settings.Credentials == empty {
		return result, nil
	}

	secretRef := repoConfig.Settings.Credentials
	key := types.NamespacedName{Namespace: secretRef.Namespace, Name: secretRef.Name}
	if err := r.Get(context.TODO(), key, &userCreatedSecret); err != nil {
		return result, errors.Wrap(err, "configured snapshot secret could not be retrieved")
	}

	err := snapshots.ValidateSnapshotCredentials(repoConfig.Type, userCreatedSecret.Data)
	if err != nil {
		return result, err
	}

	var settings []keystore.Setting
	for k := range userCreatedSecret.Data {
		settings = append(
			settings,
			keystore.Setting{
				Key:           snapshots.RepositoryCredentialsKey(repoConfig),
				ValueFilePath: path.Join(elasticsearch.KeystoreSecretMountPath, k),
			})
	}
	result.KeystoreSettings = settings
	result.KeystoreSecretRef = secretRef
	return result, nil

}

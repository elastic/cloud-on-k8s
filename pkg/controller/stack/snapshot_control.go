package stack

import (
	"context"
	"fmt"
	"path"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch"
	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/keystore"
	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/snapshots"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (r *ReconcileStack) ReconcileSnapshotCredentials(repoConfig deploymentsv1alpha1.SnapshotRepository) (keystore.Config, error) {

	log.Info(fmt.Sprintf("Snapshot repo is %v", repoConfig))

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

	//TODO proper validation
	if len(userCreatedSecret.Data) != 1 {
		return result, errors.New("Secret specified in snapshot repository needs to contain exactly one data element")
	}

	var settings []keystore.Setting
	for k, _ := range userCreatedSecret.Data {
		settings = append(
			settings,
			keystore.Setting{
				Key:           snapshots.RepositoryCredentialsKey(repoConfig),
				ValueFilePath: path.Join(elasticsearch.KeystoreSecretMountPath, k),
			})
	}
	result.KeystoreSettings = settings
	result.KeystoreSecretRef = secretRef
	log.Info(fmt.Sprintf("Keystore init will be %v", result))
	return result, nil

}

package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/watches"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/configmap"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/nodecerts"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/settings"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/snapshot"
	"github.com/elastic/stack-operators/stack-operator/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// VersionWideResources are resources that are tied to a version, but no specific pod within that version
type VersionWideResources struct {
	// ExtraFilesSecret contains some extra files we want to have access to in the containers, but had nowhere we wanted
	// it to call home, so they ended up here.
	ExtraFilesSecret corev1.Secret
	// GenericUnecryptedConfigurationFiles contains non-secret files Pods with this version should have access to.
	GenericUnecryptedConfigurationFiles corev1.ConfigMap
	// KeyStoreConfig are secrets that should go into the ES keystore
	KeyStoreConfig corev1.Secret
}

func reconcileVersionWideResources(
	c client.Client,
	scheme *runtime.Scheme,
	es v1alpha1.ElasticsearchCluster,
	trustRelationships []v1alpha1.TrustRelationship,
	w watches.DynamicWatches,
) (*VersionWideResources, error) {
	keystoreConfig, err := snapshot.ReconcileSnapshotCredentials(c, scheme, es, es.Spec.SnapshotRepository, w)
	if err != nil {
		return nil, err
	}

	expectedConfigMap := configmap.NewConfigMapWithData(es, settings.DefaultConfigMapData)
	err = configmap.ReconcileConfigMap(c, scheme, es, expectedConfigMap)
	if err != nil {
		return nil, err
	}

	// TODO: suffix and trim
	extraFilesSecretObjectKey := types.NamespacedName{
		Namespace: es.Namespace,
		Name:      fmt.Sprintf("%s-extrafiles", es.Name),
	}
	var extraFilesSecret corev1.Secret
	if err := c.Get(
		context.TODO(),
		extraFilesSecretObjectKey,
		&extraFilesSecret,
	); err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}

	trustRootCfg := nodecerts.NewTrustRootConfig(es.Name, es.Namespace)

	// include the trust restrictions from the trust relationships info the trust restrictions config
	for _, trustRelationship := range trustRelationships {
		trustRootCfg.Include(trustRelationship.Spec.TrustRestrictions)
	}

	trustRootCfgData, err := json.Marshal(&trustRootCfg)
	if err != nil {
		return nil, err
	}

	if apierrors.IsNotFound(err) {
		extraFilesSecret = corev1.Secret{
			ObjectMeta: k8s.ToObjectMeta(extraFilesSecretObjectKey),
			Data: map[string][]byte{
				nodecerts.TrustRestrictionsFilename: trustRootCfgData,
			},
		}

		err = controllerutil.SetControllerReference(&es, &extraFilesSecret, scheme)
		if err != nil {
			return nil, err
		}

		log.Info("Creating version wide resources", "clusterName", es.ClusterName)

		if err := c.Create(context.TODO(), &extraFilesSecret); err != nil {
			return nil, err
		}
	} else {
		currentTrustConfig, ok := extraFilesSecret.Data[nodecerts.TrustRestrictionsFilename]
		extraFilesSecret.Data[nodecerts.TrustRestrictionsFilename] = trustRootCfgData

		if !ok || !bytes.Equal(currentTrustConfig, trustRootCfgData) {
			log.Info("Updating version wide resources", "clusterName", es.ClusterName)

			if err := c.Update(context.TODO(), &extraFilesSecret); err != nil {
				return nil, err
			}
		}
	}

	return &VersionWideResources{
		KeyStoreConfig:                      keystoreConfig,
		GenericUnecryptedConfigurationFiles: expectedConfigMap,
		ExtraFilesSecret:                    extraFilesSecret,
	}, nil
}

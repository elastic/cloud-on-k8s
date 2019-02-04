package driver

import (
	"encoding/json"
	"fmt"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/nodecerts"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/watches"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/configmap"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/settings"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/snapshot"
	"github.com/elastic/stack-operators/stack-operator/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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
	c k8s.Client,
	scheme *runtime.Scheme,
	es v1alpha1.ElasticsearchCluster,
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
		extraFilesSecretObjectKey,
		&extraFilesSecret,
	); err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	} else if apierrors.IsNotFound(err) {
		// TODO: handle reconciling Data section if it already exists
		trustRootCfg := nodecerts.NewTrustRootConfig(es.Name, es.Namespace)
		trustRootCfgData, err := json.Marshal(&trustRootCfg)
		if err != nil {
			return nil, err
		}

		extraFilesSecret = corev1.Secret{
			ObjectMeta: k8s.ToObjectMeta(extraFilesSecretObjectKey),
			Data: map[string][]byte{
				"trust.yml": trustRootCfgData,
			},
		}

		err = controllerutil.SetControllerReference(&es, &extraFilesSecret, scheme)
		if err != nil {
			return nil, err
		}

		if err := c.Create(&extraFilesSecret); err != nil {
			return nil, err
		}
	}

	return &VersionWideResources{
		KeyStoreConfig:                      keystoreConfig,
		GenericUnecryptedConfigurationFiles: expectedConfigMap,
		ExtraFilesSecret:                    extraFilesSecret,
	}, nil
}

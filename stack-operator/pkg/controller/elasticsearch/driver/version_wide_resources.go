package driver

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/reconciler"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/watches"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/configmap"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/nodecerts"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/settings"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/snapshot"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

	trustRootCfg := nodecerts.NewTrustRootConfig(es.Name, es.Namespace)

	// include the trust restrictions from the trust relationships info the trust restrictions config
	for _, trustRelationship := range trustRelationships {
		trustRootCfg.Include(trustRelationship.Spec.TrustRestrictions)
	}

	trustRootCfgData, err := json.Marshal(&trustRootCfg)
	if err != nil {
		return nil, err
	}

	expectedExtraFilesSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			// TODO: suffix and trim
			Name: fmt.Sprintf("%s-extrafiles", es.Name),
		},
		Data: map[string][]byte{
			nodecerts.TrustRestrictionsFilename: trustRootCfgData,
		},
	}

	var reconciledExtraFilesSecret corev1.Secret

	if err := reconciler.ReconcileResource(reconciler.Params{
		Client:     c,
		Scheme:     scheme,
		Owner:      &es,
		Expected:   &expectedExtraFilesSecret,
		Reconciled: &reconciledExtraFilesSecret,
		NeedsUpdate: func() bool {
			// .Data might be nil in the secret, so make sure to initialize it
			if reconciledExtraFilesSecret.Data == nil {
				reconciledExtraFilesSecret.Data = make(map[string][]byte, 1)
			}
			currentTrustConfig, ok := reconciledExtraFilesSecret.Data[nodecerts.TrustRestrictionsFilename]

			return !ok || !bytes.Equal(currentTrustConfig, trustRootCfgData)
		},
		UpdateReconciled: func() {
			reconciledExtraFilesSecret.Data[nodecerts.TrustRestrictionsFilename] = trustRootCfgData
		},
	}); err != nil {
		return nil, err
	}

	return &VersionWideResources{
		KeyStoreConfig:                      keystoreConfig,
		GenericUnecryptedConfigurationFiles: expectedConfigMap,
		ExtraFilesSecret:                    reconciledExtraFilesSecret,
	}, nil
}

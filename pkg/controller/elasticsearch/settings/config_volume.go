// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

import (
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	common "github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
)

// Constants to use for the `elasticsearch.yml` config file in an ES pod.
const (
	ConfigFileName        = "elasticsearch.yml"
	ConfigVolumeName      = "elastic-internal-elasticsearch-config"
	ConfigVolumeMountPath = "/mnt/elastic-internal/elasticsearch-config"
)

// ConfigSecretName is the name of the secret that holds the ES config for the given StatefulSet.
func ConfigSecretName(ssetName string) string {
	return name.ConfigSecret(ssetName)
}

// ConfigSecretVolume returns a SecretVolume to hold the config of nodes in the given stateful set..
func ConfigSecretVolume(ssetName string) volume.SecretVolume {
	return volume.NewSecretVolumeWithMountPath(
		ConfigSecretName(ssetName),
		ConfigVolumeName,
		ConfigVolumeMountPath,
	)
}

// GetESConfigContent retrieves the configuration secret of the given stateful set,
// and returns the corresponding CanonicalConfig.
func GetESConfigContent(client k8s.Client, namespace string, ssetName string) (CanonicalConfig, error) {
	secret, err := GetESConfigSecret(client, namespace, ssetName)
	if err != nil {
		return CanonicalConfig{}, err
	}
	if len(secret.Data) == 0 {
		return CanonicalConfig{}, fmt.Errorf("no configuration found in secret %s", ConfigSecretName(ssetName))
	}
	content := secret.Data[ConfigFileName]
	if len(content) == 0 {
		return CanonicalConfig{}, fmt.Errorf("no configuration found in secret %s", ConfigSecretName(ssetName))
	}

	cfg, err := common.ParseConfig(content)
	if err != nil {
		return CanonicalConfig{}, err
	}
	return CanonicalConfig{cfg}, nil
}

// GetESConfigSecret returns the secret holding the ES configuration for the given pod
func GetESConfigSecret(client k8s.Client, namespace string, ssetName string) (corev1.Secret, error) {
	var secret corev1.Secret
	if err := client.Get(types.NamespacedName{
		Namespace: namespace,
		Name:      ConfigSecretName(ssetName),
	}, &secret); err != nil {
		return corev1.Secret{}, err
	}
	return secret, nil
}

// ReconcileConfig ensures the ES config for the pod is set in the apiserver.
func ReconcileConfig(client k8s.Client, es v1alpha1.Elasticsearch, ssetName string, config CanonicalConfig) error {
	rendered, err := config.Render()
	if err != nil {
		return err
	}
	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      ConfigSecretName(ssetName),
			Labels:    label.NewConfigLabels(k8s.ExtractNamespacedName(&es), ssetName),
		},
		Data: map[string][]byte{
			ConfigFileName: rendered,
		},
	}
	reconciled := corev1.Secret{}
	if err := reconciler.ReconcileResource(reconciler.Params{
		Client:   client,
		Expected: &expected,
		NeedsUpdate: func() bool {
			return !reflect.DeepEqual(reconciled.Data, expected.Data)
		},
		Owner:            &es,
		Reconciled:       &reconciled,
		Scheme:           scheme.Scheme,
		UpdateReconciled: func() { reconciled.Data = expected.Data },
	}); err != nil {
		return err
	}
	return nil
}

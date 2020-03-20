// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

import (
	pkgerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	common "github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// Constants to use for the `elasticsearch.yml` config file in an ES pod.
const (
	ConfigFileName        = "elasticsearch.yml"
	ConfigVolumeName      = "elastic-internal-elasticsearch-config"
	ConfigVolumeMountPath = "/mnt/elastic-internal/elasticsearch-config"
)

// ConfigSecretName is the name of the secret that holds the ES config for the given StatefulSet.
func ConfigSecretName(ssetName string) string {
	return esv1.ConfigSecret(ssetName)
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
		return CanonicalConfig{}, pkgerrors.Errorf("no configuration found in secret %s", ConfigSecretName(ssetName))
	}
	content := secret.Data[ConfigFileName]
	if len(content) == 0 {
		return CanonicalConfig{}, pkgerrors.Errorf("no configuration found in secret %s", ConfigSecretName(ssetName))
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

func ConfigSecret(es esv1.Elasticsearch, ssetName string, configData []byte) corev1.Secret {
	return corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      ConfigSecretName(ssetName),
			Labels:    label.NewConfigLabels(k8s.ExtractNamespacedName(&es), ssetName),
		},
		Data: map[string][]byte{
			ConfigFileName: configData,
		},
	}
}

// ReconcileConfig ensures the ES config for the pod is set in the apiserver.
func ReconcileConfig(client k8s.Client, es esv1.Elasticsearch, ssetName string, config CanonicalConfig) error {
	rendered, err := config.Render()
	if err != nil {
		return err
	}
	expected := ConfigSecret(es, ssetName, rendered)
	_, err = reconciler.ReconcileSecret(client, expected, &es)
	return err
}

// DeleteConfig removes the configuration Secret corresponding to the given Statefulset.
func DeleteConfig(client k8s.Client, namespace string, ssetName string) error {
	// build a dummy config with no data but the correct Namespace & Name,
	// to target the correct resource for deletion
	cfgSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      ConfigSecretName(ssetName),
		},
	}
	return client.Delete(&cfgSecret)
}

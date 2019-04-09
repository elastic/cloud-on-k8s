// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

import (
	"fmt"
	"reflect"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/reconciler"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/volume"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
)

// Constants to use for the `elasticsearch.yml` config file in an ES pod.
const (
	ConfigFileName        = "elasticsearch.yml"
	ConfigVolumeName      = "es-config"
	ConfigVolumeMountPath = "/mnt/elastic/" + ConfigVolumeName
)

// ConfigSecretName is the name of the secret that holds the ES config for the given pod.
func ConfigSecretName(podName string) string {
	return name.Suffix(podName, name.ConfigSecretSuffix)
}

// ConfigSecretVolume returns a SecretVolume to hold the config of the given pod.
func ConfigSecretVolume(podName string) volume.SecretVolume {
	return volume.NewSecretVolumeWithMountPath(
		ConfigSecretName(podName),
		ConfigVolumeName,
		ConfigVolumeMountPath,
	)
}

// GetESConfigContent retrieves the configuration secret of the given pod,
// and returns the corresponding FlatConfig.
func GetESConfigContent(client k8s.Client, esPod types.NamespacedName) (FlatConfig, error) {
	secret, err := GetESConfigSecret(client, esPod)
	if err != nil {
		return FlatConfig{}, err
	}
	if len(secret.Data) == 0 {
		return FlatConfig{}, fmt.Errorf("no configuration found in secret %s", ConfigSecretName(esPod.Name))
	}
	content := secret.Data[ConfigFileName]
	if len(content) == 0 {
		return FlatConfig{}, fmt.Errorf("no configuration found in secret %s", ConfigSecretName(esPod.Name))
	}

	return ParseConfig(string(content))
}

// GetESConfigSecret returns the secret holding the ES configuration for the given pod
func GetESConfigSecret(client k8s.Client, esPod types.NamespacedName) (corev1.Secret, error) {
	var secret corev1.Secret
	if err := client.Get(types.NamespacedName{
		Namespace: esPod.Namespace,
		Name:      ConfigSecretName(esPod.Name),
	}, &secret); err != nil {
		return corev1.Secret{}, err
	}
	return secret, nil
}

// ReconcileConfig ensures the ES config for the pod is set in the apiserver.
func ReconcileConfig(client k8s.Client, cluster v1alpha1.Elasticsearch, pod corev1.Pod, config FlatConfig) error {
	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: pod.Namespace,
			Name:      ConfigSecretName(pod.Name),
			Labels: map[string]string{
				label.ClusterNameLabelName: cluster.Name,
				label.PodNameLabelName:     pod.Name,
			},
		},
		Data: map[string][]byte{
			ConfigFileName: config.Render(),
		},
	}
	reconciled := corev1.Secret{}
	if err := reconciler.ReconcileResource(reconciler.Params{
		Client:   client,
		Expected: &expected,
		NeedsUpdate: func() bool {
			return !reflect.DeepEqual(reconciled.Data, expected.Data)
		},
		Owner:            &cluster,
		Reconciled:       &reconciled,
		Scheme:           scheme.Scheme,
		UpdateReconciled: func() { reconciled.Data = expected.Data },
	}); err != nil {
		return err
	}
	return nil
}

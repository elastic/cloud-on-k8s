// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stackmon

import (
	"crypto/sha256"
	"fmt"
	"hash"
	"path/filepath"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// beatConfig helps to create a beat configuration
type beatConfig struct {
	filepath string
	hash     hash.Hash
	secret   corev1.Secret
	volumes  []volume.VolumeLike
}

func newBeatConfig(client k8s.Client, beatName string, resource HasMonitoring, associations []commonv1.Association, baseConfig string) (beatConfig, error) {
	if len(associations) != 1 {
		// should never happen because of the pre-creation validation
		return beatConfig{}, errors.New("only one Elasticsearch reference is supported for Stack Monitoring")
	}
	assoc := associations[0]

	// build the output section of the beat configuration file
	outputCfg, caVolume, err := buildOutputConfig(client, assoc)
	if err != nil {
		return beatConfig{}, err
	}
	outputConfig := map[string]interface{}{
		"output": map[string]interface{}{
			"elasticsearch": outputCfg,
		},
	}

	// name for the config secret and the associated config volume for the es pod
	configName := configVolumeName(resource.GetName(), beatName)
	configFilename := fmt.Sprintf("%s.yml", beatName)
	configDirPath := fmt.Sprintf("/etc/%s-config", beatName)

	// add the config volume
	configVolume := volume.NewSecretVolumeWithMountPath(configName, configName, configDirPath)
	configFilepath := filepath.Join(configDirPath, configFilename)
	volumes := []volume.VolumeLike{configVolume}

	// add the CA volume
	if caVolume != nil {
		volumes = append(volumes, caVolume)
	}

	// merge the base config with the generated part
	configBytes, err := mergeConfig(baseConfig, outputConfig)
	if err != nil {
		return beatConfig{}, err
	}

	configHash := sha256.New224()
	configHash.Write(configBytes)

	configSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configName,
			Namespace: resource.GetNamespace(),
			Labels:    label.NewLabels(k8s.ExtractNamespacedName(resource)),
		},
		Data: map[string][]byte{
			configFilename: configBytes,
		},
	}

	return beatConfig{
		filepath: configFilepath,
		hash:     configHash,
		secret:   configSecret,
		volumes:  volumes,
	}, err
}

func buildOutputConfig(client k8s.Client, assoc commonv1.Association) (map[string]interface{}, volume.VolumeLike, error) {
	username, password, err := association.ElasticsearchAuthSettings(client, assoc)
	if err != nil {
		return nil, volume.SecretVolume{}, err
	}

	outputConfig := map[string]interface{}{
		"username": username,
		"password": password,
		"hosts":    []string{assoc.AssociationConf().GetURL()},
	}

	caDirPath := fmt.Sprintf(
		"/mnt/elastic-internal/%s-association/%s/%s/certs",
		assoc.AssociationType(), assoc.AssociationRef().Namespace, assoc.AssociationRef().Name,
	)

	var caVolume volume.VolumeLike
	if assoc.AssociationConf().GetCACertProvided() {
		sslCAPath := filepath.Join(caDirPath, certificates.CAFileName)
		outputConfig["ssl.certificate_authorities"] = []string{sslCAPath}
		volumeName := caVolumeName(assoc)
		caVolume = volume.NewSecretVolumeWithMountPath(
			assoc.AssociationConf().GetCASecretName(), volumeName, caDirPath,
		)
	}

	return outputConfig, caVolume, nil
}

func mergeConfig(rawConfig string, config map[string]interface{}) ([]byte, error) {
	cfg, err := settings.ParseConfig([]byte(rawConfig))
	if err != nil {
		return nil, err
	}

	outputCfg, err := settings.NewCanonicalConfigFrom(config)
	if err != nil {
		return nil, err
	}

	err = cfg.MergeWith(outputCfg)
	if err != nil {
		return nil, err
	}

	cfgBytes, err := cfg.Render()
	if err != nil {
		return nil, err
	}

	return cfgBytes, nil
}

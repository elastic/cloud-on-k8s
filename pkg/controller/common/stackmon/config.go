// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackmon

import (
	"bytes"
	"fmt"
	"hash"
	"hash/fnv"
	"path/filepath"
	"text/template"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/stackmon/monitoring"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// beatConfig helps to create a beat configuration
type beatConfig struct {
	filepath string
	hash     hash.Hash
	secret   corev1.Secret
	volumes  []volume.VolumeLike
}

func newBeatConfig(client k8s.Client, beatName string, resource monitoring.HasMonitoring, associations []commonv1.Association, baseConfig string) (beatConfig, error) {
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
	configSecretName := fmt.Sprintf("%s-%s-%s-config", resource.GetName(), string(assoc.AssociationType()), beatName)
	configName := configVolumeName(resource.GetName(), beatName)
	configFilename := fmt.Sprintf("%s.yml", beatName)
	configDirPath := fmt.Sprintf("/etc/%s-config", beatName)

	// add the config volume
	configVolume := volume.NewSecretVolumeWithMountPath(configSecretName, configName, configDirPath)
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

	configHash := fnv.New32a()
	configHash.Write(configBytes)

	configSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configSecretName,
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
	credentials, err := association.ElasticsearchAuthSettings(client, assoc)
	if err != nil {
		return nil, volume.SecretVolume{}, err
	}

	assocConf, err := assoc.AssociationConf()
	if err != nil {
		return nil, nil, err
	}
	outputConfig := map[string]interface{}{
		"username": credentials.Username,
		"password": credentials.Password,
		"hosts":    []string{assocConf.GetURL()},
	}

	caDirPath := fmt.Sprintf(
		"/mnt/elastic-internal/%s-association/%s/%s/certs",
		assoc.AssociationType(), assoc.AssociationRef().Namespace, assoc.AssociationRef().Name,
	)

	var caVolume volume.VolumeLike
	if assocConf.GetCACertProvided() {
		sslCAPath := filepath.Join(caDirPath, certificates.CAFileName)
		outputConfig["ssl.certificate_authorities"] = []string{sslCAPath}
		volumeName := caVolumeName(assoc)
		caVolume = volume.NewSecretVolumeWithMountPath(
			assocConf.GetCASecretName(), volumeName, caDirPath,
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

// inputConfigData holds data to configure the Metricbeat Elasticsearch and Kibana modules used
// to collect metrics for Stack Monitoring
type inputConfigData struct {
	URL      string
	Username string
	Password string
	IsSSL    bool
	HasCA    bool
	CAPath   string
}

// buildMetricbeatBaseConfig builds the base configuration for Metricbeat with the Elasticsearch or Kibana modules used
// to collect metrics for Stack Monitoring
func buildMetricbeatBaseConfig(
	client k8s.Client,
	associationType commonv1.AssociationType,
	nsn types.NamespacedName,
	esNsn types.NamespacedName,
	namer name.Namer,
	url string,
	isTLS bool,
	configTemplate string,
) (string, volume.VolumeLike, error) {
	password, err := user.GetMonitoringUserPassword(client, esNsn)
	if err != nil {
		return "", nil, err
	}

	hasCA := false
	if isTLS {
		var err error
		hasCA, err = certificates.PublicCertsHasCACert(client, namer, nsn.Namespace, nsn.Name)
		if err != nil {
			return "", nil, err
		}
	}

	configData := inputConfigData{
		Username: user.MonitoringUserName,
		Password: password,
		URL:      url,   // Metricbeat in the sidecar connects to the monitored resource using `localhost`
		IsSSL:    isTLS, // enable SSL configuration based on whether the monitored resource has TLS enabled
		HasCA:    hasCA, // the CA is optional to support custom certificate issued by a well-known CA, so without provided CA to configure
	}

	var caVolume volume.VolumeLike
	if configData.HasCA {
		caVolume = volume.NewSecretVolumeWithMountPath(
			certificates.PublicCertsSecretName(namer, nsn.Name),
			fmt.Sprintf("%s-local-ca", string(associationType)),
			fmt.Sprintf("/mnt/elastic-internal/%s/%s/%s/certs", string(associationType), nsn.Namespace, nsn.Name),
		)

		configData.CAPath = filepath.Join(caVolume.VolumeMount().MountPath, certificates.CAFileName)
	}

	// render the config template with the config data
	var metricbeatConfig bytes.Buffer
	err = template.Must(template.New("").Parse(configTemplate)).Execute(&metricbeatConfig, configData)
	if err != nil {
		return "", nil, err
	}

	return metricbeatConfig.String(), caVolume, nil
}

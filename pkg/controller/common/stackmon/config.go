// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackmon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash"
	"hash/fnv"
	"path/filepath"
	"text/template"

	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/stackmon/monitoring"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// beatConfig helps to create a beat configuration
type beatConfig struct {
	filepath string
	hash     hash.Hash
	secret   corev1.Secret
	volumes  []volume.VolumeLike
}

func newBeatConfig(
	ctx context.Context,
	client k8s.Client,
	beatName string,
	imageVersion string,
	resource monitoring.HasMonitoring,
	associations []commonv1.Association,
	baseConfig string,
	meta metadata.Metadata,
) (beatConfig, error) {
	if len(associations) != 1 {
		// should never happen because of the pre-creation validation
		return beatConfig{}, errors.New("only one Elasticsearch reference is supported for Stack Monitoring")
	}
	assoc := associations[0]

	// build the output section of the beat configuration file
	outputCfg, caVolume, err := buildOutputConfig(ctx, client, assoc, imageVersion)
	if err != nil {
		return beatConfig{}, err
	}
	outputConfig := map[string]any{
		"output": map[string]any{
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

	_, err = configHash.Write(configBytes)
	if err != nil {
		return beatConfig{}, err
	}

	meta = meta.Merge(metadata.Metadata{Labels: resource.GetIdentityLabels()})
	configSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        configSecretName,
			Namespace:   resource.GetNamespace(),
			Labels:      meta.Labels,
			Annotations: meta.Annotations,
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

func buildOutputConfig(ctx context.Context, client k8s.Client, assoc commonv1.Association, imageVersion string) (map[string]any, volume.VolumeLike, error) {
	credentials, err := association.ElasticsearchAuthSettings(ctx, client, assoc)
	if err != nil {
		return nil, volume.SecretVolume{}, err
	}

	assocConf, err := assoc.AssociationConf()
	if err != nil {
		return nil, nil, err
	}
	outputConfig := map[string]any{
		"username": credentials.Username,
		"password": credentials.Password,
		"hosts":    []string{assocConf.GetURL()},
	}

	// Elasticsearch certificate might have been generated for a "public" hostname,
	// and therefore not being valid for the internal URL.
	outputConfig["ssl.verification_mode"] = "certificate"

	v, err := version.Parse(imageVersion)
	if err != nil {
		return nil, nil, err
	}
	// Reloading of certificates is only supported for Beats >= 8.8.0.
	if v.GE(version.MinFor(8, 8, 0)) {
		// Allow beats to reload when the ssl certificate changes (renewals)
		outputConfig["ssl.restart_on_cert_change"] = map[string]any{
			"enabled": true,
			"period":  "1m",
		}
	}

	caDirPath := fmt.Sprintf(
		"/mnt/elastic-internal/%s-association/%s/%s/certs",
		assoc.AssociationType(), assoc.AssociationRef().Namespace, assoc.AssociationRef().NameOrSecretName(),
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

func mergeConfig(rawConfig string, config map[string]any) ([]byte, error) {
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

func RenderTemplate(v semver.Version, configTemplate string, params any) (string, error) {
	// render the config template with the config data
	var beatConfig bytes.Buffer
	err := template.Must(template.New("").Funcs(TemplateFuncs(v)).Parse(configTemplate)).Execute(&beatConfig, params)
	if err != nil {
		return "", err
	}
	return beatConfig.String(), nil
}

func TemplateFuncs(
	version semver.Version,
) template.FuncMap {
	return template.FuncMap{
		"sanitizeJSON": func(v any) (string, error) {
			b, err := json.Marshal(v)
			if err != nil {
				return "", fmt.Errorf("json marshal failed: %w", err)
			}
			return string(b), nil
		},
		"isVersionGTE": func(minAllowedVersion string) (bool, error) {
			minAllowedSemver, err := semver.Parse(minAllowedVersion)
			if err != nil {
				return false, err
			}
			return version.GTE(minAllowedSemver), nil
		},
		"CAPath": func(caVolume volume.VolumeLike) string {
			if caVolume == nil {
				return ""
			}
			return filepath.Join(caVolume.VolumeMount().MountPath, certificates.CAFileName)
		},
	}
}

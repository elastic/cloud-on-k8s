// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"hash"
	"path"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

type DefaultConfig struct {
	// Managed config is handled by ECK and will not be nullified by user provided config.
	Managed *settings.CanonicalConfig
	// Unmanaged config is default config that will be overridden by any user provided config.
	Unmanaged *settings.CanonicalConfig
}

// buildOutputConfig will create the output section in Beat config according to the association configuration.
func buildOutputConfig(client k8s.Client, associated beatv1beta1.BeatESAssociation) (*settings.CanonicalConfig, error) {
	if !associated.AssociationConf().IsConfigured() {
		return settings.NewCanonicalConfig(), nil
	}

	username, password, err := association.ElasticsearchAuthSettings(client, &associated)
	if err != nil {
		return settings.NewCanonicalConfig(), err
	}

	esOutput := map[string]interface{}{
		"output.elasticsearch": map[string]interface{}{
			"hosts":    []string{associated.AssociationConf().GetURL()},
			"username": username,
			"password": password,
		},
	}

	if associated.AssociationConf().GetCACertProvided() {
		esOutput["output.elasticsearch.ssl.certificate_authorities"] = []string{path.Join(certificatesDir(&associated), CAFileName)}
	}

	return settings.NewCanonicalConfigFrom(esOutput)
}

// BuildKibanaConfig builds on optional Kibana configuration for dashboard setup and visualizations.
func BuildKibanaConfig(client k8s.Client, associated beatv1beta1.BeatKibanaAssociation) (*settings.CanonicalConfig, error) {
	if !associated.AssociationConf().IsConfigured() {
		return settings.NewCanonicalConfig(), nil
	}

	username, password, err := association.ElasticsearchAuthSettings(client, &associated)
	if err != nil {
		return settings.NewCanonicalConfig(), err
	}

	kibanaCfg := map[string]interface{}{
		"setup.dashboards.enabled": true,
		"setup.kibana": map[string]interface{}{
			"host":     associated.AssociationConf().GetURL(),
			"username": username,
			"password": password,
		},
	}

	if associated.AssociationConf().GetCACertProvided() {
		kibanaCfg["setup.kibana.ssl.certificate_authorities"] = []string{path.Join(certificatesDir(&associated), CAFileName)}
	}
	return settings.NewCanonicalConfigFrom(kibanaCfg)
}

func buildBeatConfig(
	log logr.Logger,
	client k8s.Client,
	beat beatv1beta1.Beat,
	defaultConfig DefaultConfig,
) ([]byte, error) {
	cfg := settings.NewCanonicalConfig()

	outputCfg, err := buildOutputConfig(client, beatv1beta1.BeatESAssociation{Beat: &beat})
	if err != nil {
		return nil, err
	}
	err = cfg.MergeWith(outputCfg, defaultConfig.Managed)
	if err != nil {
		return nil, err
	}
	// use only the default config or only the provided config - no overriding, no merging
	userConfig := beat.Spec.Config
	if userConfig == nil {
		if err := cfg.MergeWith(defaultConfig.Unmanaged); err != nil {
			return nil, err
		}
	} else {
		userCfg, err := settings.NewCanonicalConfigFrom(userConfig.Data)
		if err != nil {
			return nil, err
		}

		if err = cfg.MergeWith(userCfg); err != nil {
			return nil, err
		}
		log.V(1).Info("Replacing ECK-managed configuration by user-provided configuration")
	}

	return cfg.Render()
}

func reconcileConfig(
	params DriverParams,
	defaultConfig DefaultConfig,
	configHash hash.Hash,
) error {
	cfgBytes, err := buildBeatConfig(params.Logger, params.Client, params.Beat, defaultConfig)
	if err != nil {
		return err
	}

	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: params.Beat.Namespace,
			Name:      ConfigSecretName(params.Beat.Spec.Type, params.Beat.Name),
			Labels:    common.AddCredentialsLabel(NewLabels(params.Beat)),
		},
		Data: map[string][]byte{
			ConfigFileName: cfgBytes,
		},
	}

	if _, err = reconciler.ReconcileSecret(params.Client, expected, &params.Beat); err != nil {
		return err
	}

	_, _ = configHash.Write(cfgBytes)

	return nil
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"hash"
	"path"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// buildOutputConfig will create the output section in Beat config according to the association configuration.
func buildOutputConfig(client k8s.Client, associated beatv1beta1.BeatESAssociation) (*settings.CanonicalConfig, error) {
	esAssocConf, err := associated.AssociationConf()
	if err != nil {
		return nil, err
	}
	if !esAssocConf.IsConfigured() {
		return settings.NewCanonicalConfig(), nil
	}

	credentials, err := association.ElasticsearchAuthSettings(client, &associated)
	if err != nil {
		return settings.NewCanonicalConfig(), err
	}

	esOutput := map[string]interface{}{
		"output.elasticsearch": map[string]interface{}{
			"hosts":    []string{esAssocConf.GetURL()},
			"username": credentials.Username,
			"password": credentials.Password,
		},
	}

	if esAssocConf.GetCACertProvided() {
		esOutput["output.elasticsearch.ssl.certificate_authorities"] = []string{path.Join(certificatesDir(&associated), CAFileName)}
	}

	return settings.NewCanonicalConfigFrom(esOutput)
}

// BuildKibanaConfig builds on optional Kibana configuration for dashboard setup and visualizations.
func BuildKibanaConfig(client k8s.Client, associated beatv1beta1.BeatKibanaAssociation) (*settings.CanonicalConfig, error) {
	kbAssocConf, err := associated.AssociationConf()
	if err != nil {
		return nil, err
	}
	if !kbAssocConf.IsConfigured() {
		return settings.NewCanonicalConfig(), nil
	}

	credentials, err := association.ElasticsearchAuthSettings(client, &associated)
	if err != nil {
		return settings.NewCanonicalConfig(), err
	}

	kibanaCfg := map[string]interface{}{
		"setup.dashboards.enabled": true,
		"setup.kibana": map[string]interface{}{
			"host":     kbAssocConf.GetURL(),
			"username": credentials.Username,
			"password": credentials.Password,
		},
	}

	if kbAssocConf.GetCACertProvided() {
		kibanaCfg["setup.kibana.ssl.certificate_authorities"] = []string{path.Join(certificatesDir(&associated), CAFileName)}
	}
	return settings.NewCanonicalConfigFrom(kibanaCfg)
}

func buildBeatConfig(
	params DriverParams,
	managedConfig *settings.CanonicalConfig,
) ([]byte, error) {
	cfg := settings.NewCanonicalConfig()

	outputCfg, err := buildOutputConfig(params.Client, beatv1beta1.BeatESAssociation{Beat: &params.Beat})
	if err != nil {
		return nil, err
	}
	err = cfg.MergeWith(outputCfg, managedConfig)
	if err != nil {
		return nil, err
	}

	// get user config from `config` or `configRef`
	userConfig, err := getUserConfig(params)
	if err != nil {
		return nil, err
	}

	if userConfig == nil {
		return cfg.Render()
	}

	if err = cfg.MergeWith(userConfig); err != nil {
		return nil, err
	}

	return cfg.Render()
}

// getUserConfig extracts the config either from the spec `config` field or from the Secret referenced by spec
// `configRef` field.
func getUserConfig(params DriverParams) (*settings.CanonicalConfig, error) {
	if params.Beat.Spec.Config != nil {
		return settings.NewCanonicalConfigFrom(params.Beat.Spec.Config.Data)
	}
	return common.ParseConfigRef(params, &params.Beat, params.Beat.Spec.ConfigRef, ConfigFileName)
}

func reconcileConfig(
	params DriverParams,
	managedConfig *settings.CanonicalConfig,
	configHash hash.Hash,
) error {
	cfgBytes, err := buildBeatConfig(params, managedConfig)
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

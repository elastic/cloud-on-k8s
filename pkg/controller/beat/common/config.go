// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"hash"
	"path"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/beat/v1beta1"
    v1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
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

	// If monitoring is enabled, render the relevant section
	if params.Beat.Spec.Monitoring.Enabled() {
		monitoringConfig, err := buildMonitoringConfig(params)
		if err != nil {
			return nil, err
		}
		if err = cfg.MergeWith(monitoringConfig); err != nil {
			return nil, err
		}
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

// buildMonitoringConfig builds the stack monitoring configuration for a Beats instance.
func buildMonitoringConfig(params DriverParams) (*settings.CanonicalConfig, error) {
	if len(params.Beat.Spec.Monitoring.ElasticsearchRefs) == 0 {
		return nil, errors.New("ElasticsearchRef must exist when stack monitoring is enabled")
	}

	// only the first ElasticsearchRef is currently supported.
	esRef := params.Beat.Spec.Monitoring.ElasticsearchRefs[0]
	if !esRef.IsDefined() {
		return nil, errors.New(validations.InvalidBeatsElasticsearchRefForStackMonitoringMsg)
	}

	var username, password, url string
	var sslConfig SSLConfig
	associations := monitoring.GetMetricsAssociation(&params.Beat)
	if len(associations) != 1 {
		// should never happen because of the pre-creation validation
		return nil, errors.New("only one Elasticsearch reference is supported for Stack Monitoring")
	}
	assoc := associations[0]

	credentials, err := association.ElasticsearchAuthSettings(params.Client, assoc)
	if err != nil {
		return nil, err
	}

	username, password = credentials.Username, credentials.Password

	var assocConf *v1.AssociationConf
	assocConf, err = assoc.AssociationConf()
	if err != nil {
		return nil, err
	}

	url = assocConf.GetURL()

	caDirPath := certificatesDir(assoc)

	if assocConf.GetCACertProvided() {
		sslCAPath := filepath.Join(caDirPath, certificates.CAFileName)
		sslConfig = SSLConfig{
			CertificateAuthorities: []string{sslCAPath},
			VerificationMode:       "certificate",
		}
	}

	config := MonitoringConfig{
		Enabled: true,
		Elasticsearch: ElasticsearchConfig{
			Hosts:    []string{url},
			Username: username,
			Password: password,
		},
	}

	if strings.Contains(url, "https") {
		config.Elasticsearch.SSL = sslConfig
	}

	return settings.NewCanonicalConfigFrom(map[string]interface{}{"monitoring": config})
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
			Labels:    labels.AddCredentialsLabel(NewLabels(params.Beat)),
		},
		Data: map[string][]byte{
			ConfigFileName: cfgBytes,
		},
	}

	if _, err = reconciler.ReconcileSecret(params.Context, params.Client, expected, &params.Beat); err != nil {
		return err
	}

	_, _ = configHash.Write(cfgBytes)

	return nil
}

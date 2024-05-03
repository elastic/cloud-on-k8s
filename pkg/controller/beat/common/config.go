// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"context"
	"encoding/base64"
	"fmt"
	"hash"
	"path"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/beat/v1beta1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/beat/common/stackmon"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/stackmon/monitoring"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

// buildOutputConfig will create the output section in Beat config according to the association configuration.
func buildOutputConfig(ctx context.Context, client k8s.Client, associated beatv1beta1.BeatESAssociation) (*settings.CanonicalConfig, error) {
	esAssocConf, err := associated.AssociationConf()
	if err != nil {
		return nil, err
	}
	if !esAssocConf.IsConfigured() {
		return settings.NewCanonicalConfig(), nil
	}

	credentials, err := association.ElasticsearchAuthSettings(ctx, client, &associated)
	if err != nil {
		return settings.NewCanonicalConfig(), err
	}

	output := map[string]interface{}{
		"hosts": []string{esAssocConf.GetURL()},
	}

	if credentials.APIKey != "" {
		decodedAPIKey, err := base64.StdEncoding.DecodeString(credentials.APIKey)
		if err != nil {
			return settings.NewCanonicalConfig(), fmt.Errorf("error at decoding apikey from secret %s: %w", esAssocConf.AuthSecretName, err)
		}
		output["api_key"] = string(decodedAPIKey)
	} else {
		output["username"] = credentials.Username
		output["password"] = credentials.Password
	}

	if esAssocConf.GetCACertProvided() {
		output["ssl.certificate_authorities"] = []string{path.Join(certificatesDir(&associated), CAFileName)}
	}

	return settings.NewCanonicalConfigFrom(map[string]interface{}{
		"output.elasticsearch": output,
	})
}

// BuildKibanaConfig builds on optional Kibana configuration for dashboard setup and visualizations.
func BuildKibanaConfig(ctx context.Context, client k8s.Client, associated beatv1beta1.BeatKibanaAssociation) (*settings.CanonicalConfig, error) {
	kbAssocConf, err := associated.AssociationConf()
	if err != nil {
		return nil, err
	}
	if !kbAssocConf.IsConfigured() {
		return settings.NewCanonicalConfig(), nil
	}

	credentials, err := association.ElasticsearchAuthSettings(ctx, client, &associated)
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

	outputCfg, err := buildOutputConfig(params.Context, params.Client, beatv1beta1.BeatESAssociation{Beat: &params.Beat})
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

	// if metrics monitoring is enabled, then
	// 1. enable the metrics http endpoint for the metricsbeat sidecar to consume
	// 2. set http.host to a unix socket
	// 3. disable http.port, as unix sockets are used to communicate
	// 4. disable internal metrics monitoring endpoint
	// 5. disable stderr, and syslog monitoring
	// 6. enable files monitoring, and configure path
	if monitoring.IsMetricsDefined(&params.Beat) {
		if err = cfg.MergeWith(settings.MustCanonicalConfig(map[string]interface{}{
			"http.enabled":       true,
			"http.host":          stackmon.GetStackMonitoringSocketURL(&params.Beat),
			"http.port":          nil,
			"monitoring.enabled": false,
			"logging.to_stderr":  false,
			"logging.to_syslog":  false,
			"logging.to_files":   true,
			"logging.files.path": "/usr/share/filebeat/logs",
		})); err != nil {
			return nil, err
		}
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
			Labels:    labels.AddCredentialsLabel(params.Beat.GetIdentityLabels()),
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

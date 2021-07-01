// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stackmon

import (
	"bytes"
	_ "embed" // for the beats config files
	"fmt"
	"path/filepath"
	"text/template"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/network"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

var (
	// metricbeatConfigTemplate is a configuration template for Metricbeat to collect monitoring data about Elasticsearch
	//go:embed metricbeat.tpl.yml
	metricbeatConfigTemplate string

	// filebeatConfig is a static configuration for Filebeat to collect Elasticsearch logs
	//go:embed filebeat.yml
	filebeatConfig string
)

// ReconcileConfigSecrets reconciles the secrets holding beats configuration
func ReconcileConfigSecrets(client k8s.Client, es esv1.Elasticsearch) error {
	if IsMonitoringMetricsDefined(es) {
		b, err := MetricbeatBuilder(client, es)
		if err != nil {
			return err
		}

		if _, err := reconciler.ReconcileSecret(client, b.ConfigSecret(), &es); err != nil {
			return err
		}
	}

	if IsMonitoringLogsDefined(es) {
		b, err := FilebeatBuilder(client, es)
		if err != nil {
			return err
		}

		if _, err := reconciler.ReconcileSecret(client, b.ConfigSecret(), &es); err != nil {
			return err
		}
	}

	return nil
}

// esConfigData holds data to configure the Metricbeat Elasticsearch module
type esConfigData struct {
	URL      string
	Username string
	Password string
	IsSSL    bool
	SSLPath  string
	SSLMode  string
}

// buildMetricbeatConfig builds the base Metricbeat config with the associated volume holding the CA of the monitored ES
func buildMetricbeatBaseConfig(client k8s.Client, es esv1.Elasticsearch) (string, volume.VolumeLike, error) {
	password, err := user.GetElasticUserPassword(client, es)
	if err != nil {
		return "", nil, err
	}

	configData := esConfigData{
		URL:      fmt.Sprintf("%s://localhost:%d", es.Spec.HTTP.Protocol(), network.HTTPPort),
		Username: user.MonitoringUserName,
		Password: password,
		IsSSL:    es.Spec.HTTP.TLS.Enabled(),
	}

	var caVolume volume.VolumeLike
	if configData.IsSSL {
		caVolume = volume.NewSecretVolumeWithMountPath(
			certificates.PublicCertsSecretName(esv1.ESNamer, es.Name),
			fmt.Sprintf("%s-%s-monitoring-local-ca", es.Name, es.ShortKind()),
			fmt.Sprintf("/mnt/elastic-internal/es-monitoring/%s/%s/certs", es.Namespace, es.Name),
		)

		configData.IsSSL = true
		configData.SSLPath = filepath.Join(caVolume.VolumeMount().MountPath, certificates.CAFileName)
		configData.SSLMode = "certificate"
	}

	// render the config template with the config data
	var metricbeatConfig bytes.Buffer
	err = template.Must(template.New("").Parse(metricbeatConfigTemplate)).Execute(&metricbeatConfig, configData)
	if err != nil {
		return "", nil, err
	}

	return metricbeatConfig.String(), caVolume, nil
}

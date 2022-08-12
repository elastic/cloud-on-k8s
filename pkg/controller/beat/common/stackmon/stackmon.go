// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackmon

import (
	_ "embed" // for the beats config files
	"fmt"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	common_name "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/stackmon"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/stackmon/monitoring"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

var (
	// filebeatConfig is a static configuration for Filebeat to collect Beats logs
	//go:embed filebeat.yml
	filebeatConfig string

	// metricbeatConfigTemplate is a configuration template for Metricbeat to collect monitoring data from Beats resources
	//go:embed metricbeat.tpl.yml
	metricbeatConfigTemplate string
)

func Filebeat(client k8s.Client, resource monitoring.HasMonitoring, version string) (stackmon.BeatSidecar, error) {
	filebeat, err := stackmon.NewFileBeatSidecar(client, resource, version, filebeatConfig, nil)
	if err != nil {
		return stackmon.BeatSidecar{}, err
	}

	return filebeat, nil
}

func MetricBeat(client k8s.Client, beat *v1beta1.Beat, version string) (stackmon.BeatSidecar, error) {
	config, err := settings.NewCanonicalConfigFrom(beat.Spec.Config.Data)
	if err != nil {
		return stackmon.BeatSidecar{}, err
	}

	// Default metricbeat monitoring port
	var httpPort uint64 = 5066
	var p httpPortSetting
	if err := config.Unpack(&p); err != nil {
		return stackmon.BeatSidecar{}, err
	}

	// if http.port is set in beats configuration, then use the port.
	if p.PortData != nil {
		portData, ok := p.PortData.(uint64)
		if !ok {
			return stackmon.BeatSidecar{}, fmt.Errorf("while configuring beats stack monitoring: 'http.port' must be an int")
		}
		httpPort = portData
	}

	sidecar, err := stackmon.NewMetricBeatSidecar(
		client,
		commonv1.BeatMonitoringAssociationType,
		beat,
		version,
		metricbeatConfigTemplate,
		common_name.NewNamer("beat"),
		fmt.Sprintf("http://localhost:%d", httpPort),
		"",
		"",
		false,
	)
	if err != nil {
		return sidecar, err
	}
	return sidecar, nil
}

type httpPortSetting struct {
	PortData interface{} `config:"http.port"`
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackmon

import (
	"bytes"
	"context"
	_ "embed" // for the beats config files
	"errors"
	"fmt"
	"text/template"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	common_name "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/stackmon"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/stackmon/monitoring"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/bootstrap"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

var (
	// filebeatConfig is a static configuration for Filebeat to collect Beats logs
	//go:embed filebeat.yml
	filebeatConfig string

	// metricbeatConfigTemplate is a configuration template for Metricbeat to collect monitoring data from Beats resources
	//go:embed metricbeat.tpl.yml
	metricbeatConfigTemplate string

	// ErrMonitoringClusterUUIDUnavailable will be returned when the UUID for the Beat ElasticsearchRef cluster
	// has not yet been assigned a UUID.  This could happen on a newly created Elasticsearch cluster.
	ErrMonitoringClusterUUIDUnavailable = errors.New("cluster UUID for Beats stack monitoring is unavailable")
)

func Filebeat(ctx context.Context, client k8s.Client, resource monitoring.HasMonitoring, version string) (stackmon.BeatSidecar, error) {
	return stackmon.NewFileBeatSidecar(ctx, client, resource, version, filebeatConfig, nil)
}

func MetricBeat(ctx context.Context, client k8s.Client, beat *v1beta1.Beat, version string) (stackmon.BeatSidecar, error) {
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

	if err := beat.ElasticsearchRef().IsValid(); err != nil {
		return stackmon.BeatSidecar{}, err
	}

	var es esv1.Elasticsearch
	if err := client.Get(ctx, beat.ElasticsearchRef().WithDefaultNamespace(beat.Namespace).NamespacedName(), &es); err != nil {
		return stackmon.BeatSidecar{}, err
	}
	uuid, ok := es.Annotations[bootstrap.ClusterUUIDAnnotationName]
	if !ok {
		// returning specific error here so this operation can be retried.
		return stackmon.BeatSidecar{}, ErrMonitoringClusterUUIDUnavailable
	}
	var beatTemplate *template.Template
	if beatTemplate, err = template.New("beat_stack_monitoring").Parse(metricbeatConfigTemplate); err != nil {
		return stackmon.BeatSidecar{}, fmt.Errorf("while parsing template for beats stack monitoring configuration: %w", err)
	}
	var byteBuffer bytes.Buffer
	data := struct {
		ClusterUUID string
		URL         string
	}{
		ClusterUUID: uuid,
		URL:         fmt.Sprintf("http://localhost:%d", httpPort),
	}
	if err := beatTemplate.Execute(&byteBuffer, data); err != nil {
		return stackmon.BeatSidecar{}, fmt.Errorf("while templating beats stack monitoring configuration: %w", err)
	}

	return stackmon.NewMetricBeatSidecar(
		ctx,
		client,
		commonv1.BeatMonitoringAssociationType,
		beat,
		version,
		byteBuffer.String(),
		common_name.NewNamer("beat"),
		fmt.Sprintf("http://localhost:%d", httpPort),
		"",
		"",
		false,
	)
}

type httpPortSetting struct {
	PortData interface{} `config:"http.port"`
}

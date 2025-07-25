// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackmon

import (
	"context"
	_ "embed" // for the beats config files
	"errors"
	"fmt"

	"github.com/elastic/cloud-on-k8s/v3/pkg/apis/beat/v1beta1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/stackmon"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/stackmon/monitoring"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/bootstrap"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

const (
	FilebeatLogsVolumeName      string = "filebeat-logs"
	FilebeatLogsVolumeMountPath string = "/usr/share/filebeat/logs"

	MetricbeatLogsVolumeName      string = "metricbeat-logs"
	MetricbeatLogsVolumeMountPath string = "/usr/share/metricbeat/logs"
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

func Filebeat(ctx context.Context, client k8s.Client, resource monitoring.HasMonitoring, version string, meta metadata.Metadata) (stackmon.BeatSidecar, error) {
	sidecar, err := stackmon.NewFileBeatSidecar(ctx, client, resource, version, filebeatConfig, nil, meta)
	if err != nil {
		return stackmon.BeatSidecar{}, err
	}

	// Add shared volume for logs consumption.
	filebeatLogsVolume := volume.NewEmptyDirVolume(FilebeatLogsVolumeName, FilebeatLogsVolumeMountPath)
	sidecar.Container.VolumeMounts = append(sidecar.Container.VolumeMounts, filebeatLogsVolume.VolumeMount())
	sidecar.Volumes = append(sidecar.Volumes, filebeatLogsVolume.Volume())

	return sidecar, nil
}

func MetricBeat(ctx context.Context, client k8s.Client, beat *v1beta1.Beat, meta metadata.Metadata) (stackmon.BeatSidecar, error) {
	if err := beat.ElasticsearchRef().IsValid(); err != nil {
		return stackmon.BeatSidecar{}, err
	}

	uuid, err := associatedESUUID(ctx, client, beat)
	if err != nil {
		return stackmon.BeatSidecar{}, err
	}

	data := struct {
		ClusterUUID string
		URL         string
	}{
		ClusterUUID: uuid,
		// https://www.elastic.co/guide/en/beats/metricbeat/current/configuration-metricbeat.html#module-http-config-options
		// Beat module http options require "http+" to be appended to unix sockets.
		URL: fmt.Sprintf("http+%s", GetStackMonitoringSocketURL(beat)),
	}
	v, err := version.Parse(beat.Spec.Version)
	if err != nil {
		return stackmon.BeatSidecar{}, err
	}
	cfg, err := stackmon.RenderTemplate(v, metricbeatConfigTemplate, data)
	if err != nil {
		return stackmon.BeatSidecar{}, err
	}

	sidecar, err := stackmon.NewMetricBeatSidecar(ctx, client, beat, v, nil, cfg, meta)
	if err != nil {
		return stackmon.BeatSidecar{}, err
	}

	// Add shared volume for Unix socket between containers.
	sharedDataVolume := volume.NewEmptyDirVolume("shared-data", "/var/shared")
	sidecar.Container.VolumeMounts = append(sidecar.Container.VolumeMounts, sharedDataVolume.VolumeMount())
	sidecar.Volumes = append(sidecar.Volumes, sharedDataVolume.Volume())

	// Add shared volume for logs consumption.
	metricbeatLogsVolume := volume.NewEmptyDirVolume(MetricbeatLogsVolumeName, MetricbeatLogsVolumeMountPath)
	sidecar.Volumes = append(sidecar.Volumes, metricbeatLogsVolume.Volume())
	sidecar.Container.VolumeMounts = append(sidecar.Container.VolumeMounts, metricbeatLogsVolume.VolumeMount())

	return sidecar, nil
}

type clusterUUIDResponse struct {
	ClusterUUID string `json:"cluster_uuid"`
}

func associatedESUUID(ctx context.Context, client k8s.Client, beat *v1beta1.Beat) (string, error) {
	esAssociation := beat.EsAssociation()
	esRef := esAssociation.AssociationRef()
	if !esRef.IsDefined() {
		// no association or indirect association e.g. via output configuration
		return "", nil
	}
	if esRef.IsExternal() {
		remoteES, err := association.GetUnmanagedAssociationConnectionInfoFromSecret(client, esAssociation)
		if err != nil {
			return "", fmt.Errorf("while retrieving external ES connection info: %w", err)
		}
		clusterUUIDResponse := &clusterUUIDResponse{}
		if err := remoteES.Request("/", clusterUUIDResponse); err != nil {
			return "", fmt.Errorf("while retrieving remote cluster UUID %w", err)
		}
		return clusterUUIDResponse.ClusterUUID, nil
	}
	var es esv1.Elasticsearch
	if err := client.Get(ctx, esRef.NamespacedName(), &es); err != nil {
		return "", err
	}
	uuid, ok := es.Annotations[bootstrap.ClusterUUIDAnnotationName]
	if !ok {
		// returning specific error here so this operation can be retried.
		return "", ErrMonitoringClusterUUIDUnavailable
	}
	return uuid, nil
}

// GetStackMonitoringSocketURL will return a path to a Unix socket that will be used to expose and query metrics.
// Unix sockets are used instead of network ports to avoid situations where "hostNetwork: true" is enabled on multiple
// Beat daemonsets, along with stack monitoring, which will cause 2 pods to try and bind to the same port on the
// Node's host network, which will cause bind errors. (bind: address already in use)
func GetStackMonitoringSocketURL(beat *v1beta1.Beat) string {
	// TODO: Enable when Beats as containers in Windows is supported: https://github.com/elastic/beats/issues/16814
	// if runtime.GOOS == "windows" {
	// 	return fmt.Sprintf("npipe:///%s-%s-%s.sock", beat.Spec.Type, beat.GetNamespace(), beat.GetName())
	// }
	return fmt.Sprintf("unix:///var/shared/%s-%s-%s.sock", beat.Spec.Type, beat.GetNamespace(), beat.GetName())
}

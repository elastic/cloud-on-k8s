// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackmon

import (
	"fmt"
	"hash/fnv"

	corev1 "k8s.io/api/core/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/stackmon"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/stackmon/monitoring"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/network"
	esvolume "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

const (
	// cfgHashAnnotation is an annotation to store a hash of the Metricbeat and Filebeat configurations to rotate the Pods when changed.
	cfgHashAnnotation = "elasticsearch.k8s.elastic.co/monitoring-config-hash"
)

func Metricbeat(client k8s.Client, es esv1.Elasticsearch) (stackmon.BeatSidecar, error) {
	metricbeat, err := stackmon.NewMetricBeatSidecar(
		client,
		commonv1.KbMonitoringAssociationType,
		&es,
		es.Spec.Version,
		k8s.ExtractNamespacedName(&es),
		metricbeatConfigTemplate,
		esv1.ESNamer,
		fmt.Sprintf("%s://localhost:%d", es.Spec.HTTP.Protocol(), network.HTTPPort),
		es.Spec.HTTP.TLS.Enabled(),
	)
	if err != nil {
		return stackmon.BeatSidecar{}, err
	}
	return metricbeat, nil
}

func Filebeat(client k8s.Client, es esv1.Elasticsearch) (stackmon.BeatSidecar, error) {
	filebeat, err := stackmon.NewFileBeatSidecar(client, &es, es.Spec.Version, filebeatConfig, nil)
	if err != nil {
		return stackmon.BeatSidecar{}, err
	}

	return filebeat, nil
}

// WithMonitoring updates the Elasticsearch Pod template builder to deploy Metricbeat and Filebeat in sidecar containers
// in the Elasticsearch pod and injects the volumes for the beat configurations and the ES CA certificates.
func WithMonitoring(client k8s.Client, builder *defaults.PodTemplateBuilder, es esv1.Elasticsearch) (*defaults.PodTemplateBuilder, error) {
	isMonitoringReconcilable, err := monitoring.IsReconcilable(&es)
	if err != nil {
		return nil, err
	}
	if !isMonitoringReconcilable {
		return builder, nil
	}

	configHash := fnv.New32a()
	volumes := make([]corev1.Volume, 0)

	if monitoring.IsMetricsDefined(&es) {
		b, err := Metricbeat(client, es)
		if err != nil {
			return nil, err
		}

		volumes = append(volumes, b.Volumes...)
		builder.WithContainers(b.Container)
		configHash.Write(b.ConfigHash.Sum(nil))
	}

	if monitoring.IsLogsDefined(&es) {
		// enable Stack logging to write Elasticsearch logs to disk
		builder.WithEnv(fileLogStyleEnvVar())

		b, err := Filebeat(client, es)
		if err != nil {
			return nil, err
		}

		volumes = append(volumes, b.Volumes...)
		filebeat := b.Container

		// share the ES logs volume into the Filebeat container
		filebeat.VolumeMounts = append(filebeat.VolumeMounts, esvolume.DefaultLogsVolumeMount)

		builder.WithContainers(filebeat)
		configHash.Write(b.ConfigHash.Sum(nil))
	}

	// add the config hash annotation to ensure pod rotation when an ES password or a CA are rotated
	builder.WithAnnotations(map[string]string{cfgHashAnnotation: fmt.Sprint(configHash.Sum32())})
	// inject all volumes
	builder.WithVolumes(volumes...)

	return builder, nil
}

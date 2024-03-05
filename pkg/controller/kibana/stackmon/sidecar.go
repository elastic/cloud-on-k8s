// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackmon

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"

	corev1 "k8s.io/api/core/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/stackmon"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/stackmon/monitoring"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/stackmon/validations"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/kibana/network"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

const (
	// cfgHashAnnotation is used to store a hash of the Metricbeat and Filebeat configurations.
	cfgHashAnnotation = "kibana.k8s.elastic.co/monitoring-config-hash"

	kibanaLogsVolumeName = "kibana-logs"
	kibanaLogsMountPath  = "/usr/share/kibana/logs"
)

func Metricbeat(ctx context.Context, client k8s.Client, kb kbv1.Kibana) (stackmon.BeatSidecar, error) {
	if !kb.Spec.ElasticsearchRef.IsDefined() {
		// should never happen because of the pre-creation validation
		return stackmon.BeatSidecar{}, errors.New(validations.InvalidKibanaElasticsearchRefForStackMonitoringMsg)
	}
	associatedEsNsn := kb.Spec.ElasticsearchRef.NamespacedName()
	if associatedEsNsn.Namespace == "" {
		associatedEsNsn.Namespace = kb.Namespace
	}

	var username, password string

	if esAssoc := kb.EsAssociation(); esAssoc.AssociationRef().IsExternal() {
		info, err := association.GetUnmanagedAssociationConnectionInfoFromSecret(client, esAssoc)
		if err != nil {
			return stackmon.BeatSidecar{}, err
		}
		username, password = info.Username, info.Password
	} else {
		var err error
		username = user.MonitoringUserName
		password, err = user.GetMonitoringUserPassword(client, associatedEsNsn)
		if err != nil {
			return stackmon.BeatSidecar{}, err
		}
	}

	metricbeat, err := stackmon.NewMetricBeatSidecar(
		ctx,
		client,
		commonv1.KbMonitoringAssociationType,
		&kb,
		kb.Spec.Version,
		metricbeatConfigTemplate,
		kbv1.KBNamer,
		fmt.Sprintf("%s://localhost:%d", kb.Spec.HTTP.Protocol(), network.HTTPPort),
		username,
		password,
		kb.Spec.HTTP.TLS.Enabled(),
	)
	if err != nil {
		return stackmon.BeatSidecar{}, err
	}
	return metricbeat, nil
}

func Filebeat(ctx context.Context, client k8s.Client, kb kbv1.Kibana) (stackmon.BeatSidecar, error) {
	return stackmon.NewFileBeatSidecar(ctx, client, &kb, kb.Spec.Version, filebeatConfig, nil)
}

// WithMonitoring updates the Kibana Pod template builder to deploy Metricbeat and Filebeat in sidecar containers
// in the Kibana pod and injects the volumes for the beat configurations and the ES CA certificates.
func WithMonitoring(ctx context.Context, client k8s.Client, builder *defaults.PodTemplateBuilder, kb kbv1.Kibana) (*defaults.PodTemplateBuilder, error) {
	isMonitoringReconcilable, err := monitoring.IsReconcilable(&kb)
	if err != nil {
		return nil, err
	}
	if !isMonitoringReconcilable {
		return builder, nil
	}

	configHash := fnv.New32a()
	volumes := make([]corev1.Volume, 0)

	if monitoring.IsMetricsDefined(&kb) {
		b, err := Metricbeat(ctx, client, kb)
		if err != nil {
			return nil, err
		}

		volumes = append(volumes, b.Volumes...)
		builder.WithContainers(b.Container)
		configHash.Write(b.ConfigHash.Sum(nil))
	}

	if monitoring.IsLogsDefined(&kb) {
		b, err := Filebeat(ctx, client, kb)
		if err != nil {
			return nil, err
		}

		// create a logs volume shared between Kibana and Filebeat
		logsVolume := volume.NewEmptyDirVolume(kibanaLogsVolumeName, kibanaLogsMountPath)
		volumes = append(volumes, logsVolume.Volume())
		filebeat := b.Container
		filebeat.VolumeMounts = append(filebeat.VolumeMounts, logsVolume.VolumeMount())
		builder.WithVolumeMounts(logsVolume.VolumeMount())

		volumes = append(volumes, b.Volumes...)
		builder.WithContainers(filebeat)
		configHash.Write(b.ConfigHash.Sum(nil))
	}

	// add the config hash annotation to ensure pod rotation when an ES password or a CA are rotated
	builder.WithAnnotations(map[string]string{cfgHashAnnotation: fmt.Sprint(configHash.Sum32())})
	// inject all volumes
	builder.WithVolumes(volumes...)

	return builder, nil
}

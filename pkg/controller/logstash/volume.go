// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"fmt"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume"
)

const (
	InitContainerConfigVolumeMountPath = "/mnt/elastic-internal/logstash-config-local"

	// InternalConfigVolumeName is a volume which contains the generated configuration.
	InternalConfigVolumeName        = "elastic-internal-logstash-config"
	InternalConfigVolumeMountPath   = "/mnt/elastic-internal/logstash-config"
	InternalPipelineVolumeName      = "elastic-internal-logstash-pipeline"
	InternalPipelineVolumeMountPath = "/mnt/elastic-internal/logstash-pipeline"
)

var (
	// ConfigSharedVolume contains the Logstash config/ directory, it contains the contents of config from the docker container
	ConfigSharedVolume = volume.SharedVolume{
		VolumeName:             ConfigVolumeName,
		InitContainerMountPath: InitContainerConfigVolumeMountPath,
		ContainerMountPath:     ConfigMountPath,
	}
)

// ConfigVolume returns a SecretVolume to hold the Logstash config of the given Logstash resource.
func ConfigVolume(ls logstashv1alpha1.Logstash) volume.SecretVolume {
	return volume.NewSecretVolumeWithMountPath(
		logstashv1alpha1.ConfigSecretName(ls.Name),
		InternalConfigVolumeName,
		InternalConfigVolumeMountPath,
	)
}

// PipelineVolume returns a SecretVolume to hold the Logstash config of the given Logstash resource.
func PipelineVolume(ls logstashv1alpha1.Logstash) volume.SecretVolume {
	return volume.NewSecretVolumeWithMountPath(
		logstashv1alpha1.PipelineSecretName(ls.Name),
		InternalPipelineVolumeName,
		InternalPipelineVolumeMountPath,
	)
}

func buildVolumes(params Params) ([]volume.VolumeLike, error) {
	vols := []volume.VolumeLike{ConfigSharedVolume, ConfigVolume(params.Logstash), PipelineVolume(params.Logstash)}

	// all volumes with CAs of direct associations
	caAssocVols, err := getVolumesFromAssociations(params.Logstash.GetAssociations())
	if err != nil {
		return nil, err
	}

	vols = append(vols, caAssocVols...)

	return vols, nil
}

func getVolumesFromAssociations(associations []commonv1.Association) ([]volume.VolumeLike, error) {
	var vols []volume.VolumeLike //nolint:prealloc
	for i, assoc := range associations {
		assocConf, err := assoc.AssociationConf()
		if err != nil {
			return nil, err
		}
		if !assocConf.CAIsConfigured() {
			// skip as there is no volume to mount if association has no CA configured
			continue
		}
		caSecretName := assocConf.GetCASecretName()
		vols = append(vols, volume.NewSecretVolumeWithMountPath(
			caSecretName,
			fmt.Sprintf("%s-certs-%d", assoc.AssociationType(), i),
			certificatesDir(assoc),
		))
	}
	return vols, nil
}

func certificatesDir(association commonv1.Association) string {
	ref := association.AssociationRef()
	return fmt.Sprintf(
		"/mnt/elastic-internal/%s-association/%s/%s/certs",
		association.AssociationType(),
		ref.Namespace,
		ref.NameOrSecretName(),
	)
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	lsvolume "github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/volume"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume"

)

const (
	ConfigVolumeName = "config"
	ConfigMountPath  = "/usr/share/logstash/config"

	LogstashConfigVolumeName = "logstash"
	LogstashConfigFileName   = "logstash.yml"

	PipelineVolumeName = "pipeline"
	PipelineFileName   = "pipelines.yml"

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


func buildVolumesAndMounts(ls logstashv1alpha1.Logstash)([]corev1.Volume, []corev1.VolumeMount) {
	persistentVolumes := make([]corev1.Volume, 0, len(ls.Spec.VolumeClaimTemplates))

	// Add logs volume
	persistentVolumes = append(persistentVolumes, lsvolume.DefaultLogsVolume)

	// Add the default `logstash-data' PVC
	ls.Spec.VolumeClaimTemplates = defaults.AppendDefaultPVCs(
		ls.Spec.VolumeClaimTemplates,
		ls.Spec.PodTemplate.Spec,
		lsvolume.DefaultVolumeClaimTemplates...,
	)

	// Add volume claims user-defined + `logstash-data`
	for _, claimTemplate := range ls.Spec.VolumeClaimTemplates {
		persistentVolumes = append(persistentVolumes, corev1.Volume{
			Name: claimTemplate.Name,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					// actual claim name will be resolved and fixed right before pod creation
					ClaimName: "claim-name-placeholder",
				},
			},
		})
	}
	persistentVolumes = append(persistentVolumes, lsvolume.DefaultLogsVolume)

	volumeMounts := make([]corev1.VolumeMount, 0)
	// Add volume mount for data volume to the total list of volumes
	// include the user-provided PodTemplate volumes as the user may have defined the data volume there (e.g.: emptyDir or hostpath volume)
	volumeMounts = lsvolume.AppendDefaultDataVolumeMount(volumeMounts, append(persistentVolumes, ls.Spec.PodTemplate.Spec.Volumes...))
	volumeMounts = append(volumeMounts, lsvolume.DefaultLogsVolumeMount)
	return persistentVolumes, volumeMounts
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

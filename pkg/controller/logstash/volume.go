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
	//"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume"

)

func buildVolumesAndMounts(ls logstashv1alpha1.Logstash)([]corev1.Volume, []corev1.VolumeMount) {
	persistentVolumes := make([]corev1.Volume, 0)

	// Create volumes from VolumeClaimTemplates.
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

	volumeMounts := make([]corev1.VolumeMount, 0)

	// Add the volume mount for the default data volume
	volumeMounts = lsvolume.AppendDefaultDataVolumeMount(volumeMounts, append(persistentVolumes, ls.Spec.PodTemplate.Spec.Volumes...))

	// Add logs volume and mount
	persistentVolumes = append(persistentVolumes, lsvolume.DefaultLogsVolume)
	volumeMounts = append(volumeMounts, lsvolume.DefaultLogsVolumeMount)

	return persistentVolumes, volumeMounts
}

func buildVolumeLikes(params Params) ([]volume.VolumeLike, error) {
	vols := []volume.VolumeLike{lsvolume.ConfigSharedVolume, lsvolume.ConfigVolume(params.Logstash), lsvolume.PipelineVolume(params.Logstash)}

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

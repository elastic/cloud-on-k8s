// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package volume

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/set"
)

// AppendDefaultPVCs appends defaults PVCs to a given list of PVCs.
// Default PVCs are appended if there is no given PVCs or volumes in the poSpec with the same name.
func AppendDefaultPVCs(existingPVCs []corev1.PersistentVolumeClaim, podSpec corev1.PodSpec) []corev1.PersistentVolumeClaim {
	// create a set of volume names
	volumeNames := set.Make()

	for _, existingPVC := range existingPVCs {
		volumeNames.Add(existingPVC.Name)
	}

	for _, existingVolume := range podSpec.Volumes {
		volumeNames.Add(existingVolume.Name)
	}

	for _, defaultPVC := range DefaultVolumeClaimTemplates {
		if volumeNames.Has(defaultPVC.Name) {
			continue
		}
		existingPVCs = append(existingPVCs, defaultPVC)
	}
	return existingPVCs
}

func BuildVolumes(ls logstashv1alpha1.Logstash, useTLS bool) ([]corev1.Volume, []corev1.VolumeMount, error) {
	// all volumes with CAs of direct associations
	volumeLikes, err := getVolumesFromAssociations(ls.GetAssociations())
	if err != nil {
		return nil, nil, err
	}

	volumeLikes = append(
		volumeLikes,
		ConfigSharedVolume,
		ConfigVolume(ls),
		PipelineVolume(ls),
		DefaultLogsVolume,
	)

	if useTLS {
		httpCertsVolume := certificates.HTTPCertSecretVolume(logstashv1alpha1.Namer, ls.Name)
		volumeLikes = append(volumeLikes, httpCertsVolume)
	}

	volumes, volumeMounts := volume.Resolve(volumeLikes)

	// append future volumes from PVCs
	for _, claimTemplate := range ls.Spec.VolumeClaimTemplates {
		volumes = append(volumes, corev1.Volume{Name: claimTemplate.Name})
	}

	// include the user-provided PodTemplate volumes as the user may have defined the data volume there (e.g.: emptyDir or hostpath volume)
	volumeMounts = AppendDefaultDataVolumeMount(volumeMounts, append(volumes, ls.Spec.PodTemplate.Spec.Volumes...))

	return volumes, volumeMounts, nil
}

func CertificatesDir(association commonv1.Association) string {
	ref := association.AssociationRef()
	return fmt.Sprintf(
		"/mnt/elastic-internal/%s-association/%s/%s/certs",
		association.AssociationType(),
		ref.Namespace,
		ref.NameOrSecretName(),
	)
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
			CertificatesDir(assoc),
		))
	}
	return vols, nil
}

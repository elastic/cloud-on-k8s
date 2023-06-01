// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package volume

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/set"
)

// AppendDefaultPVCs appends defaults PVCs to a set of existing ones.
//
// The default PVCs are not appended if:
// - a Volume with the same .Name is found in podSpec.Volumes, and that volume is not a PVC volume
func AppendDefaultPVCs(existing []corev1.PersistentVolumeClaim, ls logstashv1alpha1.Logstash) []corev1.PersistentVolumeClaim {
	// create a set of volume names that are not PVC-volumes
	existingVolumes := set.Make()

	for _, existingPVC := range existing {
		existingVolumes.Add(existingPVC.Name)
	}

	for _, volume := range ls.Spec.PodTemplate.Spec.Volumes {
		existingVolumes.Add(volume.Name)
	}

	for _, defaultPVC := range DefaultVolumeClaimTemplates {
		if existingVolumes.Has(defaultPVC.Name) {
			continue
		}
		existing = append(existing, defaultPVC)
	}
	return existing
}

func BuildVolumesAndMounts(ls logstashv1alpha1.Logstash) ([]corev1.Volume, []corev1.VolumeMount) {
	persistentVolumes := make([]corev1.Volume, 0)

	// Add Default logs volume to list of persistent volumes
	persistentVolumes = append(persistentVolumes, DefaultLogsVolume)

	// Create volumes from any VolumeClaimTemplates.
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

	// Add volume mounts for data and log volumes
	volumeMounts = AppendDefaultDataVolumeMount(volumeMounts, append(persistentVolumes, ls.Spec.PodTemplate.Spec.Volumes...))
	volumeMounts = AppendDefaultLogVolumeMount(volumeMounts, append(persistentVolumes, ls.Spec.PodTemplate.Spec.Volumes...))

	return persistentVolumes, volumeMounts
}

func BuildVolumeLikes(ls logstashv1alpha1.Logstash) ([]volume.VolumeLike, error) {
	vols := []volume.VolumeLike{ConfigSharedVolume, ConfigVolume(ls), PipelineVolume(ls)}

	// all volumes with CAs of direct associations
	caAssocVols, err := getVolumesFromAssociations(ls.GetAssociations())
	if err != nil {
		return nil, err
	}

	vols = append(vols, caAssocVols...)

	return vols, nil
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

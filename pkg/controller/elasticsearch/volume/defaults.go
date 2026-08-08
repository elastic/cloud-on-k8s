// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package volume

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	DefaultPersistentVolumeSize = resource.MustParse("1Gi")

	// DefaultDataVolumeClaim is the default data volume claim for Elasticsearch pods.
	// We default to a 1GB persistent volume, using the default storage class.
	DefaultDataVolumeClaim = corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: ElasticsearchDataVolumeName,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: DefaultPersistentVolumeSize,
				},
			},
		},
	}
	DefaultDataVolumeMount = corev1.VolumeMount{
		Name:      ElasticsearchDataVolumeName,
		MountPath: ElasticsearchDataMountPath,
	}

	// DefaultVolumeClaimTemplates is the default volume claim templates for Elasticsearch pods
	DefaultVolumeClaimTemplates = []corev1.PersistentVolumeClaim{DefaultDataVolumeClaim}

	// DefaultLogsVolume is the default EmptyDir logs volume for Elasticsearch pods.
	DefaultLogsVolume = corev1.Volume{
		Name: ElasticsearchLogsVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
	// DefaultLogsVolumeMount is the default logs volume mount for the Elasticsearch container.
	DefaultLogsVolumeMount = corev1.VolumeMount{
		Name:      ElasticsearchLogsVolumeName,
		MountPath: ElasticsearchLogsMountPath,
	}
)

// ApplyStorageOverride returns claims with the storage request of the data claim set to storage.
//
// The data claim is the one named ElasticsearchDataVolumeName if present, and otherwise the single
// claim when exactly one is declared: a NodeSet managed by an autoscaling policy is allowed at most
// one claim, and that claim may carry a custom name.
//
// A nil storage means "do not override" and returns the input untouched, as does a claim list that
// is empty or that holds several claims none of which is the data claim. Otherwise the claims are
// deep-copied, so the result shares no state with the input: callers hold a NodeSet copied by
// value, whose slice header still points at the Elasticsearch resource's backing array.
func ApplyStorageOverride(claims []corev1.PersistentVolumeClaim, storage *resource.Quantity) []corev1.PersistentVolumeClaim {
	if storage == nil || len(claims) == 0 {
		return claims
	}

	target := -1
	for i := range claims {
		if claims[i].Name == ElasticsearchDataVolumeName {
			target = i
			break
		}
	}
	if target < 0 {
		if len(claims) != 1 {
			return claims
		}
		target = 0
	}

	out := make([]corev1.PersistentVolumeClaim, len(claims))
	for i := range claims {
		claims[i].DeepCopyInto(&out[i])
	}
	if out[target].Spec.Resources.Requests == nil {
		out[target].Spec.Resources.Requests = corev1.ResourceList{}
	}
	out[target].Spec.Resources.Requests[corev1.ResourceStorage] = storage.DeepCopy()
	return out
}

// AppendDefaultDataVolumeMount appends a volume mount for the default data volume if the slice of volumes contains the default data volume.
func AppendDefaultDataVolumeMount(mounts []corev1.VolumeMount, volumes []corev1.Volume) []corev1.VolumeMount {
	for _, v := range volumes {
		if v.Name == ElasticsearchDataVolumeName {
			return append(mounts, DefaultDataVolumeMount)
		}
	}
	return mounts
}

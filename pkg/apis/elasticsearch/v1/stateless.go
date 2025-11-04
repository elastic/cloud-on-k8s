// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/volume"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
)

type ElasticsearchTierName string

const (
	MLTierName     ElasticsearchTierName = "ml"
	SearchTierName ElasticsearchTierName = "search"
	IndexTierName  ElasticsearchTierName = "index"
)

var AllElasticsearchTierNames = []ElasticsearchTierName{
	IndexTierName,
	SearchTierName,
	MLTierName,
}

type StatelessSpec struct {
	// +kubebuilder:validation:Required
	Tiers Tiers `json:"tiers"`

	// +kubebuilder:validation:Required
	StatelessConfig StatelessConfig `json:"config,omitempty"`
}

type StatelessConfig struct {
	// +kubebuilder:validation:Required
	ObjectStore ObjectStoreConfig `json:"object_store,omitempty"`
}

type ObjectStoreConfig struct {
	// +kubebuilder:validation:Optional
	BasePath string `json:"base_path,omitempty"`

	// +kubebuilder:validation:Required
	Bucket string `json:"bucket,omitempty"`

	// +kubebuilder:validation:Required
	Client string `json:"client,omitempty"`

	// +kubebuilder:validation:Required
	Type string `json:"type,omitempty"`
}

type Tiers struct {
	Index  TierSpec `json:"index"`
	Search TierSpec `json:"search"`
	ML     TierSpec `json:"ml"`
}

func (es *Elasticsearch) GetTierSpec(tierName ElasticsearchTierName) (*TierSpec, error) {
	switch tierName {
	case IndexTierName:
		return &es.Spec.StatelessSpec.Tiers.Index, nil
	case SearchTierName:
		return &es.Spec.StatelessSpec.Tiers.Search, nil
	case MLTierName:
		return &es.Spec.StatelessSpec.Tiers.ML, nil
	default:
		return nil, fmt.Errorf("unknown tier name: %s", tierName)
	}
}

type TierSpec struct {
	// Count is the desired number of pods in this tier.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Required
	Count int32 `json:"count"`

	// PodTemplate is the pod template to use for the pods in this tier.
	// +kubebuilder:validation:Optional
	// +kubebuilder:pruning:PreserveUnknownFields
	PodTemplate corev1.PodTemplateSpec `json:"podTemplate,omitempty"`

	// Config holds the Elasticsearch configuration specific to a tier.
	// +kubebuilder:validation:Optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Config *commonv1.Config `json:"config,omitempty"`

	// VolumeClaimTemplate is the volume claim template to use for the caching volume in this tier.
	// +kubebuilder:validation:Optional
	// +kubebuilder:pruning:PreserveUnknownFields
	VolumeClaimTemplate *VolumeClaimTemplate `json:"volumeClaimTemplate,omitempty"`
}

// +kubebuilder:object:generate=false
type NamedTierSpec struct {
	Name string
	*TierSpec
}

func (t *TierSpec) AsNamedTierSpec(name ElasticsearchTierName) *NamedTierSpec {
	return &NamedTierSpec{
		Name:     string(name),
		TierSpec: t,
	}
}

func (nt *NamedTierSpec) GetName() string {
	if nt == nil || nt.TierSpec == nil {
		return ""
	}
	return nt.Name
}

func (nt *NamedTierSpec) GetVolumeClaimTemplates() []corev1.PersistentVolumeClaim {
	if nt == nil || nt.TierSpec == nil {
		return nil
	}
	pvct := nt.VolumeClaimTemplate.ToPersistentVolumeClaimTemplate()
	if pvct == nil {
		return nil
	}
	return []corev1.PersistentVolumeClaim{*pvct}
}

func (nt *NamedTierSpec) GetPodTemplate() corev1.PodTemplateSpec {
	if nt == nil || nt.TierSpec == nil {
		return corev1.PodTemplateSpec{}
	}
	return nt.PodTemplate
}

type VolumeClaimTemplate struct {
	Metadata SimpleMetadata `json:"metadata,omitempty"`

	// spec defines the desired characteristics of a volume requested by a pod author.
	// More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#persistentvolumeclaims
	// +kubebuilder:validation:Required
	Spec corev1.PersistentVolumeClaimSpec `json:"spec,omitempty"`
}

func (vct *VolumeClaimTemplate) ToPersistentVolumeClaimTemplate() *corev1.PersistentVolumeClaim {
	if vct == nil {
		defaultSpec := volume.DefaultDataVolumeClaim.Spec.DeepCopy()
		defaultVolumeClaimTemplate := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: volume.ElasticsearchDataVolumeName,
			},
			Spec: *defaultSpec,
		}
		return defaultVolumeClaimTemplate
	}

	result := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: vct.Metadata.Annotations,
			Labels:      vct.Metadata.Labels,
			Name:        volume.ElasticsearchDataVolumeName,
		},
		Spec: vct.Spec,
	}
	if result.Spec.AccessModes == nil {
		result.Spec.AccessModes = volume.DefaultDataVolumeClaim.Spec.AccessModes
	}
	return result
}

type SimpleMetadata struct {
	Annotations map[string]string `json:"annotations,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

type PodTemplate struct {
	Spec corev1.PodTemplateSpec `json:"podTemplate,omitempty"`

	// TopologySpreadConstraintsTemplate defines the topology spread constraints to be applied to the pods.
	// When defined, the operator automatically adds the right labelSelector to math the Pods for the Deployment.
	// +kubebuilder:validation:Optional
	TopologySpreadConstraintsTemplate TopologySpreadConstraintsTemplate `json:"topologySpreadConstraintsTemplate,omitempty"`
}

type TopologySpreadConstraintsTemplate struct {
	*corev1.TopologySpreadConstraint `json:",inline"`
}

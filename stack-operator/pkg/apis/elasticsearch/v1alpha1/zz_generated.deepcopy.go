// +build !ignore_autogenerated

// Code generated by main. DO NOT EDIT.

package v1alpha1

import (
	commonv1alpha1 "github.com/elastic/stack-operators/stack-operator/pkg/apis/common/v1alpha1"
	v1 "k8s.io/api/core/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ChangeBudget) DeepCopyInto(out *ChangeBudget) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ChangeBudget.
func (in *ChangeBudget) DeepCopy() *ChangeBudget {
	if in == nil {
		return nil
	}
	out := new(ChangeBudget)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterLicense) DeepCopyInto(out *ClusterLicense) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	out.Status = in.Status
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterLicense.
func (in *ClusterLicense) DeepCopy() *ClusterLicense {
	if in == nil {
		return nil
	}
	out := new(ClusterLicense)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ClusterLicense) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterLicenseList) DeepCopyInto(out *ClusterLicenseList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ClusterLicense, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterLicenseList.
func (in *ClusterLicenseList) DeepCopy() *ClusterLicenseList {
	if in == nil {
		return nil
	}
	out := new(ClusterLicenseList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ClusterLicenseList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterLicenseSpec) DeepCopyInto(out *ClusterLicenseSpec) {
	*out = *in
	out.SignatureRef = in.SignatureRef
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterLicenseSpec.
func (in *ClusterLicenseSpec) DeepCopy() *ClusterLicenseSpec {
	if in == nil {
		return nil
	}
	out := new(ClusterLicenseSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterLicenseStatus) DeepCopyInto(out *ClusterLicenseStatus) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterLicenseStatus.
func (in *ClusterLicenseStatus) DeepCopy() *ClusterLicenseStatus {
	if in == nil {
		return nil
	}
	out := new(ClusterLicenseStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ElasticsearchCluster) DeepCopyInto(out *ElasticsearchCluster) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ElasticsearchCluster.
func (in *ElasticsearchCluster) DeepCopy() *ElasticsearchCluster {
	if in == nil {
		return nil
	}
	out := new(ElasticsearchCluster)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ElasticsearchCluster) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ElasticsearchClusterList) DeepCopyInto(out *ElasticsearchClusterList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ElasticsearchCluster, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ElasticsearchClusterList.
func (in *ElasticsearchClusterList) DeepCopy() *ElasticsearchClusterList {
	if in == nil {
		return nil
	}
	out := new(ElasticsearchClusterList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ElasticsearchClusterList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ElasticsearchPodSpec) DeepCopyInto(out *ElasticsearchPodSpec) {
	*out = *in
	if in.Affinity != nil {
		in, out := &in.Affinity, &out.Affinity
		*out = new(v1.Affinity)
		(*in).DeepCopyInto(*out)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ElasticsearchPodSpec.
func (in *ElasticsearchPodSpec) DeepCopy() *ElasticsearchPodSpec {
	if in == nil {
		return nil
	}
	out := new(ElasticsearchPodSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ElasticsearchPodTemplateSpec) DeepCopyInto(out *ElasticsearchPodTemplateSpec) {
	*out = *in
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ElasticsearchPodTemplateSpec.
func (in *ElasticsearchPodTemplateSpec) DeepCopy() *ElasticsearchPodTemplateSpec {
	if in == nil {
		return nil
	}
	out := new(ElasticsearchPodTemplateSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ElasticsearchSpec) DeepCopyInto(out *ElasticsearchSpec) {
	*out = *in
	if in.Topologies != nil {
		in, out := &in.Topologies, &out.Topologies
		*out = make([]ElasticsearchTopologySpec, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.SnapshotRepository != nil {
		in, out := &in.SnapshotRepository, &out.SnapshotRepository
		*out = new(SnapshotRepository)
		**out = **in
	}
	if in.FeatureFlags != nil {
		in, out := &in.FeatureFlags, &out.FeatureFlags
		*out = make(commonv1alpha1.FeatureFlags, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	in.UpdateStrategy.DeepCopyInto(&out.UpdateStrategy)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ElasticsearchSpec.
func (in *ElasticsearchSpec) DeepCopy() *ElasticsearchSpec {
	if in == nil {
		return nil
	}
	out := new(ElasticsearchSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ElasticsearchStatus) DeepCopyInto(out *ElasticsearchStatus) {
	*out = *in
	out.ReconcilerStatus = in.ReconcilerStatus
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ElasticsearchStatus.
func (in *ElasticsearchStatus) DeepCopy() *ElasticsearchStatus {
	if in == nil {
		return nil
	}
	out := new(ElasticsearchStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ElasticsearchTopologySpec) DeepCopyInto(out *ElasticsearchTopologySpec) {
	*out = *in
	out.NodeTypes = in.NodeTypes
	in.Resources.DeepCopyInto(&out.Resources)
	in.PodTemplate.DeepCopyInto(&out.PodTemplate)
	if in.VolumeClaimTemplates != nil {
		in, out := &in.VolumeClaimTemplates, &out.VolumeClaimTemplates
		*out = make([]v1.PersistentVolumeClaim, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ElasticsearchTopologySpec.
func (in *ElasticsearchTopologySpec) DeepCopy() *ElasticsearchTopologySpec {
	if in == nil {
		return nil
	}
	out := new(ElasticsearchTopologySpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EnterpriseLicense) DeepCopyInto(out *EnterpriseLicense) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EnterpriseLicense.
func (in *EnterpriseLicense) DeepCopy() *EnterpriseLicense {
	if in == nil {
		return nil
	}
	out := new(EnterpriseLicense)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *EnterpriseLicense) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EnterpriseLicenseList) DeepCopyInto(out *EnterpriseLicenseList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]EnterpriseLicense, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EnterpriseLicenseList.
func (in *EnterpriseLicenseList) DeepCopy() *EnterpriseLicenseList {
	if in == nil {
		return nil
	}
	out := new(EnterpriseLicenseList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *EnterpriseLicenseList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EnterpriseLicenseSpec) DeepCopyInto(out *EnterpriseLicenseSpec) {
	*out = *in
	out.SignatureRef = in.SignatureRef
	if in.ClusterLicenses != nil {
		in, out := &in.ClusterLicenses, &out.ClusterLicenses
		*out = make([]ClusterLicense, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EnterpriseLicenseSpec.
func (in *EnterpriseLicenseSpec) DeepCopy() *EnterpriseLicenseSpec {
	if in == nil {
		return nil
	}
	out := new(EnterpriseLicenseSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EnterpriseLicenseStatus) DeepCopyInto(out *EnterpriseLicenseStatus) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EnterpriseLicenseStatus.
func (in *EnterpriseLicenseStatus) DeepCopy() *EnterpriseLicenseStatus {
	if in == nil {
		return nil
	}
	out := new(EnterpriseLicenseStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GroupingDefinition) DeepCopyInto(out *GroupingDefinition) {
	*out = *in
	in.Selector.DeepCopyInto(&out.Selector)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GroupingDefinition.
func (in *GroupingDefinition) DeepCopy() *GroupingDefinition {
	if in == nil {
		return nil
	}
	out := new(GroupingDefinition)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NodeTypesSpec) DeepCopyInto(out *NodeTypesSpec) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NodeTypesSpec.
func (in *NodeTypesSpec) DeepCopy() *NodeTypesSpec {
	if in == nil {
		return nil
	}
	out := new(NodeTypesSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SafetyMargin) DeepCopyInto(out *SafetyMargin) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SafetyMargin.
func (in *SafetyMargin) DeepCopy() *SafetyMargin {
	if in == nil {
		return nil
	}
	out := new(SafetyMargin)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SnapshotRepository) DeepCopyInto(out *SnapshotRepository) {
	*out = *in
	out.Settings = in.Settings
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SnapshotRepository.
func (in *SnapshotRepository) DeepCopy() *SnapshotRepository {
	if in == nil {
		return nil
	}
	out := new(SnapshotRepository)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SnapshotRepositorySettings) DeepCopyInto(out *SnapshotRepositorySettings) {
	*out = *in
	out.Credentials = in.Credentials
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SnapshotRepositorySettings.
func (in *SnapshotRepositorySettings) DeepCopy() *SnapshotRepositorySettings {
	if in == nil {
		return nil
	}
	out := new(SnapshotRepositorySettings)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *UpdateStrategy) DeepCopyInto(out *UpdateStrategy) {
	*out = *in
	if in.Groups != nil {
		in, out := &in.Groups, &out.Groups
		*out = make([]GroupingDefinition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.ChangeBudget != nil {
		in, out := &in.ChangeBudget, &out.ChangeBudget
		*out = new(ChangeBudget)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new UpdateStrategy.
func (in *UpdateStrategy) DeepCopy() *UpdateStrategy {
	if in == nil {
		return nil
	}
	out := new(UpdateStrategy)
	in.DeepCopyInto(out)
	return out
}

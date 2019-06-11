// +build !ignore_autogenerated

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// Code generated by main. DO NOT EDIT.

package v1alpha1

import (
	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
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
func (in *ClusterLicenseSpec) DeepCopyInto(out *ClusterLicenseSpec) {
	*out = *in
	out.LicenseMeta = in.LicenseMeta
	in.SignatureRef.DeepCopyInto(&out.SignatureRef)
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
func (in *ClusterSettings) DeepCopyInto(out *ClusterSettings) {
	*out = *in
	if in.InitialMasterNodes != nil {
		in, out := &in.InitialMasterNodes, &out.InitialMasterNodes
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterSettings.
func (in *ClusterSettings) DeepCopy() *ClusterSettings {
	if in == nil {
		return nil
	}
	out := new(ClusterSettings)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Elasticsearch) DeepCopyInto(out *Elasticsearch) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Elasticsearch.
func (in *Elasticsearch) DeepCopy() *Elasticsearch {
	if in == nil {
		return nil
	}
	out := new(Elasticsearch)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Elasticsearch) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ElasticsearchList) DeepCopyInto(out *ElasticsearchList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Elasticsearch, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ElasticsearchList.
func (in *ElasticsearchList) DeepCopy() *ElasticsearchList {
	if in == nil {
		return nil
	}
	out := new(ElasticsearchList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ElasticsearchList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ElasticsearchSettings) DeepCopyInto(out *ElasticsearchSettings) {
	*out = *in
	out.Node = in.Node
	in.Cluster.DeepCopyInto(&out.Cluster)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ElasticsearchSettings.
func (in *ElasticsearchSettings) DeepCopy() *ElasticsearchSettings {
	if in == nil {
		return nil
	}
	out := new(ElasticsearchSettings)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ElasticsearchSpec) DeepCopyInto(out *ElasticsearchSpec) {
	*out = *in
	if in.SetVMMaxMapCount != nil {
		in, out := &in.SetVMMaxMapCount, &out.SetVMMaxMapCount
		*out = new(bool)
		**out = **in
	}
	in.HTTP.DeepCopyInto(&out.HTTP)
	if in.Nodes != nil {
		in, out := &in.Nodes, &out.Nodes
		*out = make([]NodeSpec, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.FeatureFlags != nil {
		in, out := &in.FeatureFlags, &out.FeatureFlags
		*out = make(commonv1alpha1.FeatureFlags, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	in.UpdateStrategy.DeepCopyInto(&out.UpdateStrategy)
	if in.PodDisruptionBudget != nil {
		in, out := &in.PodDisruptionBudget, &out.PodDisruptionBudget
		*out = new(commonv1alpha1.PodDisruptionBudgetTemplate)
		(*in).DeepCopyInto(*out)
	}
	if in.SecureSettings != nil {
		in, out := &in.SecureSettings, &out.SecureSettings
		*out = new(commonv1alpha1.SecretRef)
		**out = **in
	}
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
	out.ZenDiscovery = in.ZenDiscovery
	if in.RemoteClusters != nil {
		in, out := &in.RemoteClusters, &out.RemoteClusters
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
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
func (in *EnterpriseLicense) DeepCopyInto(out *EnterpriseLicense) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
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
	out.LicenseMeta = in.LicenseMeta
	in.SignatureRef.DeepCopyInto(&out.SignatureRef)
	if in.ClusterLicenseSpecs != nil {
		in, out := &in.ClusterLicenseSpecs, &out.ClusterLicenseSpecs
		*out = make([]ClusterLicenseSpec, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	out.Eula = in.Eula
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
func (in *EulaState) DeepCopyInto(out *EulaState) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EulaState.
func (in *EulaState) DeepCopy() *EulaState {
	if in == nil {
		return nil
	}
	out := new(EulaState)
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
func (in *LicenseMeta) DeepCopyInto(out *LicenseMeta) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LicenseMeta.
func (in *LicenseMeta) DeepCopy() *LicenseMeta {
	if in == nil {
		return nil
	}
	out := new(LicenseMeta)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LocalRefStatus) DeepCopyInto(out *LocalRefStatus) {
	*out = *in
	out.RemoteSelector = in.RemoteSelector
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LocalRefStatus.
func (in *LocalRefStatus) DeepCopy() *LocalRefStatus {
	if in == nil {
		return nil
	}
	out := new(LocalRefStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Node) DeepCopyInto(out *Node) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Node.
func (in *Node) DeepCopy() *Node {
	if in == nil {
		return nil
	}
	out := new(Node)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NodeSpec) DeepCopyInto(out *NodeSpec) {
	*out = *in
	if in.Config != nil {
		in, out := &in.Config, &out.Config
		*out = (*in).DeepCopy()
	}
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

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NodeSpec.
func (in *NodeSpec) DeepCopy() *NodeSpec {
	if in == nil {
		return nil
	}
	out := new(NodeSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RemoteCluster) DeepCopyInto(out *RemoteCluster) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	in.Status.DeepCopyInto(&out.Status)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RemoteCluster.
func (in *RemoteCluster) DeepCopy() *RemoteCluster {
	if in == nil {
		return nil
	}
	out := new(RemoteCluster)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *RemoteCluster) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RemoteClusterList) DeepCopyInto(out *RemoteClusterList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]RemoteCluster, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RemoteClusterList.
func (in *RemoteClusterList) DeepCopy() *RemoteClusterList {
	if in == nil {
		return nil
	}
	out := new(RemoteClusterList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *RemoteClusterList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RemoteClusterRef) DeepCopyInto(out *RemoteClusterRef) {
	*out = *in
	out.K8sLocalRef = in.K8sLocalRef
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RemoteClusterRef.
func (in *RemoteClusterRef) DeepCopy() *RemoteClusterRef {
	if in == nil {
		return nil
	}
	out := new(RemoteClusterRef)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RemoteClusterSpec) DeepCopyInto(out *RemoteClusterSpec) {
	*out = *in
	out.Remote = in.Remote
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RemoteClusterSpec.
func (in *RemoteClusterSpec) DeepCopy() *RemoteClusterSpec {
	if in == nil {
		return nil
	}
	out := new(RemoteClusterSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RemoteClusterStatus) DeepCopyInto(out *RemoteClusterStatus) {
	*out = *in
	if in.SeedHosts != nil {
		in, out := &in.SeedHosts, &out.SeedHosts
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	out.K8SLocalStatus = in.K8SLocalStatus
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RemoteClusterStatus.
func (in *RemoteClusterStatus) DeepCopy() *RemoteClusterStatus {
	if in == nil {
		return nil
	}
	out := new(RemoteClusterStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TrustRelationship) DeepCopyInto(out *TrustRelationship) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TrustRelationship.
func (in *TrustRelationship) DeepCopy() *TrustRelationship {
	if in == nil {
		return nil
	}
	out := new(TrustRelationship)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *TrustRelationship) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TrustRelationshipList) DeepCopyInto(out *TrustRelationshipList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]TrustRelationship, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TrustRelationshipList.
func (in *TrustRelationshipList) DeepCopy() *TrustRelationshipList {
	if in == nil {
		return nil
	}
	out := new(TrustRelationshipList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *TrustRelationshipList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TrustRelationshipSpec) DeepCopyInto(out *TrustRelationshipSpec) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TrustRelationshipSpec.
func (in *TrustRelationshipSpec) DeepCopy() *TrustRelationshipSpec {
	if in == nil {
		return nil
	}
	out := new(TrustRelationshipSpec)
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

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ZenDiscoveryStatus) DeepCopyInto(out *ZenDiscoveryStatus) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ZenDiscoveryStatus.
func (in *ZenDiscoveryStatus) DeepCopy() *ZenDiscoveryStatus {
	if in == nil {
		return nil
	}
	out := new(ZenDiscoveryStatus)
	in.DeepCopyInto(out)
	return out
}

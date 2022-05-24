//go:build !ignore_autogenerated

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// Code generated by controller-gen. DO NOT EDIT.

package v1beta1

import (
	"github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EnterpriseSearch) DeepCopyInto(out *EnterpriseSearch) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
	if in.assocConf != nil {
		in, out := &in.assocConf, &out.assocConf
		*out = new(v1.AssociationConf)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EnterpriseSearch.
func (in *EnterpriseSearch) DeepCopy() *EnterpriseSearch {
	if in == nil {
		return nil
	}
	out := new(EnterpriseSearch)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *EnterpriseSearch) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EnterpriseSearchList) DeepCopyInto(out *EnterpriseSearchList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]EnterpriseSearch, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EnterpriseSearchList.
func (in *EnterpriseSearchList) DeepCopy() *EnterpriseSearchList {
	if in == nil {
		return nil
	}
	out := new(EnterpriseSearchList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *EnterpriseSearchList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EnterpriseSearchSpec) DeepCopyInto(out *EnterpriseSearchSpec) {
	*out = *in
	if in.Config != nil {
		in, out := &in.Config, &out.Config
		*out = (*in).DeepCopy()
	}
	if in.ConfigRef != nil {
		in, out := &in.ConfigRef, &out.ConfigRef
		*out = new(v1.ConfigSource)
		**out = **in
	}
	in.HTTP.DeepCopyInto(&out.HTTP)
	out.ElasticsearchRef = in.ElasticsearchRef
	in.PodTemplate.DeepCopyInto(&out.PodTemplate)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EnterpriseSearchSpec.
func (in *EnterpriseSearchSpec) DeepCopy() *EnterpriseSearchSpec {
	if in == nil {
		return nil
	}
	out := new(EnterpriseSearchSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EnterpriseSearchStatus) DeepCopyInto(out *EnterpriseSearchStatus) {
	*out = *in
	out.DeploymentStatus = in.DeploymentStatus
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EnterpriseSearchStatus.
func (in *EnterpriseSearchStatus) DeepCopy() *EnterpriseSearchStatus {
	if in == nil {
		return nil
	}
	out := new(EnterpriseSearchStatus)
	in.DeepCopyInto(out)
	return out
}

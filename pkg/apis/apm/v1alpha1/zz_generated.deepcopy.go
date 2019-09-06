// +build !ignore_autogenerated

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// Code generated by main. DO NOT EDIT.

package v1alpha1

import (
	commonv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1alpha1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ApmServer) DeepCopyInto(out *ApmServer) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
	if in.assocConf != nil {
		in, out := &in.assocConf, &out.assocConf
		*out = new(commonv1alpha1.AssociationConf)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ApmServer.
func (in *ApmServer) DeepCopy() *ApmServer {
	if in == nil {
		return nil
	}
	out := new(ApmServer)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ApmServer) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ApmServerList) DeepCopyInto(out *ApmServerList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ApmServer, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ApmServerList.
func (in *ApmServerList) DeepCopy() *ApmServerList {
	if in == nil {
		return nil
	}
	out := new(ApmServerList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ApmServerList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ApmServerSpec) DeepCopyInto(out *ApmServerSpec) {
	*out = *in
	if in.Config != nil {
		in, out := &in.Config, &out.Config
		*out = (*in).DeepCopy()
	}
	in.HTTP.DeepCopyInto(&out.HTTP)
	out.ElasticsearchRef = in.ElasticsearchRef
	in.PodTemplate.DeepCopyInto(&out.PodTemplate)
	if in.SecureSettings != nil {
		in, out := &in.SecureSettings, &out.SecureSettings
		*out = make([]commonv1alpha1.SecretRef, len(*in))
		copy(*out, *in)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ApmServerSpec.
func (in *ApmServerSpec) DeepCopy() *ApmServerSpec {
	if in == nil {
		return nil
	}
	out := new(ApmServerSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ApmServerStatus) DeepCopyInto(out *ApmServerStatus) {
	*out = *in
	out.ReconcilerStatus = in.ReconcilerStatus
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ApmServerStatus.
func (in *ApmServerStatus) DeepCopy() *ApmServerStatus {
	if in == nil {
		return nil
	}
	out := new(ApmServerStatus)
	in.DeepCopyInto(out)
	return out
}

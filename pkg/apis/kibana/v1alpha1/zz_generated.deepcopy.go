// +build !ignore_autogenerated

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	commonv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1alpha1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BackendElasticsearch) DeepCopyInto(out *BackendElasticsearch) {
	*out = *in
	in.Auth.DeepCopyInto(&out.Auth)
	out.CertificateAuthorities = in.CertificateAuthorities
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BackendElasticsearch.
func (in *BackendElasticsearch) DeepCopy() *BackendElasticsearch {
	if in == nil {
		return nil
	}
	out := new(BackendElasticsearch)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Kibana) DeepCopyInto(out *Kibana) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Kibana.
func (in *Kibana) DeepCopy() *Kibana {
	if in == nil {
		return nil
	}
	out := new(Kibana)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Kibana) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *KibanaList) DeepCopyInto(out *KibanaList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Kibana, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new KibanaList.
func (in *KibanaList) DeepCopy() *KibanaList {
	if in == nil {
		return nil
	}
	out := new(KibanaList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *KibanaList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *KibanaSpec) DeepCopyInto(out *KibanaSpec) {
	*out = *in
	out.ElasticsearchRef = in.ElasticsearchRef
	in.Elasticsearch.DeepCopyInto(&out.Elasticsearch)
	if in.Config != nil {
		in, out := &in.Config, &out.Config
		*out = (*in).DeepCopy()
	}
	in.HTTP.DeepCopyInto(&out.HTTP)
	in.PodTemplate.DeepCopyInto(&out.PodTemplate)
	if in.SecureSettings != nil {
		in, out := &in.SecureSettings, &out.SecureSettings
		*out = new(commonv1alpha1.SecretRef)
		**out = **in
	}
	if in.FeatureFlags != nil {
		in, out := &in.FeatureFlags, &out.FeatureFlags
		*out = make(commonv1alpha1.FeatureFlags, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new KibanaSpec.
func (in *KibanaSpec) DeepCopy() *KibanaSpec {
	if in == nil {
		return nil
	}
	out := new(KibanaSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *KibanaStatus) DeepCopyInto(out *KibanaStatus) {
	*out = *in
	out.ReconcilerStatus = in.ReconcilerStatus
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new KibanaStatus.
func (in *KibanaStatus) DeepCopy() *KibanaStatus {
	if in == nil {
		return nil
	}
	out := new(KibanaStatus)
	in.DeepCopyInto(out)
	return out
}

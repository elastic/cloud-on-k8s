// +build !ignore_autogenerated

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// Code generated by main. DO NOT EDIT.

package v1alpha1

import (
	commonv1alpha1 "github.com/elastic/k8s-operators/operators/pkg/apis/common/v1alpha1"
	v1 "k8s.io/api/core/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BackendElasticsearch) DeepCopyInto(out *BackendElasticsearch) {
	*out = *in
	in.Auth.DeepCopyInto(&out.Auth)
	if in.CaCertSecret != nil {
		in, out := &in.CaCertSecret, &out.CaCertSecret
		*out = new(string)
		**out = **in
	}
	return
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
func (in *ElasticsearchAuth) DeepCopyInto(out *ElasticsearchAuth) {
	*out = *in
	if in.Inline != nil {
		in, out := &in.Inline, &out.Inline
		*out = new(ElasticsearchInlineAuth)
		**out = **in
	}
	if in.SecretKeyRef != nil {
		in, out := &in.SecretKeyRef, &out.SecretKeyRef
		*out = new(v1.SecretKeySelector)
		(*in).DeepCopyInto(*out)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ElasticsearchAuth.
func (in *ElasticsearchAuth) DeepCopy() *ElasticsearchAuth {
	if in == nil {
		return nil
	}
	out := new(ElasticsearchAuth)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ElasticsearchInlineAuth) DeepCopyInto(out *ElasticsearchInlineAuth) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ElasticsearchInlineAuth.
func (in *ElasticsearchInlineAuth) DeepCopy() *ElasticsearchInlineAuth {
	if in == nil {
		return nil
	}
	out := new(ElasticsearchInlineAuth)
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
	return
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
	return
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
	in.Resources.DeepCopyInto(&out.Resources)
	if in.FeatureFlags != nil {
		in, out := &in.FeatureFlags, &out.FeatureFlags
		*out = make(commonv1alpha1.FeatureFlags, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	return
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
	return
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

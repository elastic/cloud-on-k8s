//go:build !ignore_autogenerated
// +build !ignore_autogenerated

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// Code generated by controller-gen. DO NOT EDIT.

package v1beta1

import (
	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Beat) DeepCopyInto(out *Beat) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
	if in.esAssocConf != nil {
		in, out := &in.esAssocConf, &out.esAssocConf
		*out = new(v1.AssociationConf)
		**out = **in
	}
	if in.kbAssocConf != nil {
		in, out := &in.kbAssocConf, &out.kbAssocConf
		*out = new(v1.AssociationConf)
		**out = **in
	}
	if in.monitoringAssocConfs != nil {
		in, out := &in.monitoringAssocConfs, &out.monitoringAssocConfs
		*out = make(map[v1.ObjectSelector]v1.AssociationConf, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Beat.
func (in *Beat) DeepCopy() *Beat {
	if in == nil {
		return nil
	}
	out := new(Beat)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Beat) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BeatESAssociation) DeepCopyInto(out *BeatESAssociation) {
	*out = *in
	if in.Beat != nil {
		in, out := &in.Beat, &out.Beat
		*out = new(Beat)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BeatESAssociation.
func (in *BeatESAssociation) DeepCopy() *BeatESAssociation {
	if in == nil {
		return nil
	}
	out := new(BeatESAssociation)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BeatKibanaAssociation) DeepCopyInto(out *BeatKibanaAssociation) {
	*out = *in
	if in.Beat != nil {
		in, out := &in.Beat, &out.Beat
		*out = new(Beat)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BeatKibanaAssociation.
func (in *BeatKibanaAssociation) DeepCopy() *BeatKibanaAssociation {
	if in == nil {
		return nil
	}
	out := new(BeatKibanaAssociation)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BeatList) DeepCopyInto(out *BeatList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Beat, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BeatList.
func (in *BeatList) DeepCopy() *BeatList {
	if in == nil {
		return nil
	}
	out := new(BeatList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *BeatList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BeatMonitoringAssociation) DeepCopyInto(out *BeatMonitoringAssociation) {
	*out = *in
	if in.Beat != nil {
		in, out := &in.Beat, &out.Beat
		*out = new(Beat)
		(*in).DeepCopyInto(*out)
	}
	out.ref = in.ref
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BeatMonitoringAssociation.
func (in *BeatMonitoringAssociation) DeepCopy() *BeatMonitoringAssociation {
	if in == nil {
		return nil
	}
	out := new(BeatMonitoringAssociation)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BeatSpec) DeepCopyInto(out *BeatSpec) {
	*out = *in
	out.ElasticsearchRef = in.ElasticsearchRef
	out.KibanaRef = in.KibanaRef
	if in.Config != nil {
		in, out := &in.Config, &out.Config
		*out = (*in).DeepCopy()
	}
	if in.ConfigRef != nil {
		in, out := &in.ConfigRef, &out.ConfigRef
		*out = new(v1.ConfigSource)
		**out = **in
	}
	if in.SecureSettings != nil {
		in, out := &in.SecureSettings, &out.SecureSettings
		*out = make([]v1.SecretSource, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.DaemonSet != nil {
		in, out := &in.DaemonSet, &out.DaemonSet
		*out = new(DaemonSetSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.Deployment != nil {
		in, out := &in.Deployment, &out.Deployment
		*out = new(DeploymentSpec)
		(*in).DeepCopyInto(*out)
	}
	in.Monitoring.DeepCopyInto(&out.Monitoring)
	if in.RevisionHistoryLimit != nil {
		in, out := &in.RevisionHistoryLimit, &out.RevisionHistoryLimit
		*out = new(int32)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BeatSpec.
func (in *BeatSpec) DeepCopy() *BeatSpec {
	if in == nil {
		return nil
	}
	out := new(BeatSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BeatStatus) DeepCopyInto(out *BeatStatus) {
	*out = *in
	if in.MonitoringAssociationsStatus != nil {
		in, out := &in.MonitoringAssociationsStatus, &out.MonitoringAssociationsStatus
		*out = make(v1.AssociationStatusMap, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BeatStatus.
func (in *BeatStatus) DeepCopy() *BeatStatus {
	if in == nil {
		return nil
	}
	out := new(BeatStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DaemonSetSpec) DeepCopyInto(out *DaemonSetSpec) {
	*out = *in
	in.PodTemplate.DeepCopyInto(&out.PodTemplate)
	in.UpdateStrategy.DeepCopyInto(&out.UpdateStrategy)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DaemonSetSpec.
func (in *DaemonSetSpec) DeepCopy() *DaemonSetSpec {
	if in == nil {
		return nil
	}
	out := new(DaemonSetSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DeploymentSpec) DeepCopyInto(out *DeploymentSpec) {
	*out = *in
	in.PodTemplate.DeepCopyInto(&out.PodTemplate)
	if in.Replicas != nil {
		in, out := &in.Replicas, &out.Replicas
		*out = new(int32)
		**out = **in
	}
	in.Strategy.DeepCopyInto(&out.Strategy)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DeploymentSpec.
func (in *DeploymentSpec) DeepCopy() *DeploymentSpec {
	if in == nil {
		return nil
	}
	out := new(DeploymentSpec)
	in.DeepCopyInto(out)
	return out
}

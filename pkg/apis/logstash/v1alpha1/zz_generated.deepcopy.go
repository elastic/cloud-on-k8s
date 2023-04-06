//go:build !ignore_autogenerated
// +build !ignore_autogenerated

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Logstash) DeepCopyInto(out *Logstash) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
	if in.MonitoringAssocConfs != nil {
		in, out := &in.MonitoringAssocConfs, &out.MonitoringAssocConfs
		*out = make(map[v1.ObjectSelector]v1.AssociationConf, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Logstash.
func (in *Logstash) DeepCopy() *Logstash {
	if in == nil {
		return nil
	}
	out := new(Logstash)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Logstash) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LogstashList) DeepCopyInto(out *LogstashList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Logstash, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LogstashList.
func (in *LogstashList) DeepCopy() *LogstashList {
	if in == nil {
		return nil
	}
	out := new(LogstashList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *LogstashList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LogstashMonitoringAssociation) DeepCopyInto(out *LogstashMonitoringAssociation) {
	*out = *in
	if in.Logstash != nil {
		in, out := &in.Logstash, &out.Logstash
		*out = new(Logstash)
		(*in).DeepCopyInto(*out)
	}
	out.ref = in.ref
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LogstashMonitoringAssociation.
func (in *LogstashMonitoringAssociation) DeepCopy() *LogstashMonitoringAssociation {
	if in == nil {
		return nil
	}
	out := new(LogstashMonitoringAssociation)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LogstashService) DeepCopyInto(out *LogstashService) {
	*out = *in
	in.Service.DeepCopyInto(&out.Service)
	in.TLS.DeepCopyInto(&out.TLS)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LogstashService.
func (in *LogstashService) DeepCopy() *LogstashService {
	if in == nil {
		return nil
	}
	out := new(LogstashService)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LogstashSpec) DeepCopyInto(out *LogstashSpec) {
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
	if in.Pipelines != nil {
		in, out := &in.Pipelines, &out.Pipelines
		*out = make([]v1.Config, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.PipelinesRef != nil {
		in, out := &in.PipelinesRef, &out.PipelinesRef
		*out = new(v1.ConfigSource)
		**out = **in
	}
	if in.Services != nil {
		in, out := &in.Services, &out.Services
		*out = make([]LogstashService, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	in.Monitoring.DeepCopyInto(&out.Monitoring)
	in.PodTemplate.DeepCopyInto(&out.PodTemplate)
	if in.RevisionHistoryLimit != nil {
		in, out := &in.RevisionHistoryLimit, &out.RevisionHistoryLimit
		*out = new(int32)
		**out = **in
	}
	if in.SecureSettings != nil {
		in, out := &in.SecureSettings, &out.SecureSettings
		*out = make([]v1.SecretSource, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LogstashSpec.
func (in *LogstashSpec) DeepCopy() *LogstashSpec {
	if in == nil {
		return nil
	}
	out := new(LogstashSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LogstashStatus) DeepCopyInto(out *LogstashStatus) {
	*out = *in
	if in.MonitoringAssociationStatus != nil {
		in, out := &in.MonitoringAssociationStatus, &out.MonitoringAssociationStatus
		*out = make(v1.AssociationStatusMap, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LogstashStatus.
func (in *LogstashStatus) DeepCopy() *LogstashStatus {
	if in == nil {
		return nil
	}
	out := new(LogstashStatus)
	in.DeepCopyInto(out)
	return out
}

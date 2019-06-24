// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apm

import (
	apmtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/apm/v1alpha1"
	common "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
)

var DefaultResources = corev1.ResourceRequirements{
	Limits: map[corev1.ResourceName]resource.Quantity{
		corev1.ResourceMemory: resource.MustParse("1G"),
	},
}

// -- Stack

type Builder struct {
	ApmServer apmtype.ApmServer
}

func (b Builder) WithRestrictedSecurityContext() Builder {
	b.ApmServer.Spec.PodTemplate.Spec.SecurityContext = helpers.DefaultSecurityContext()
	return b
}

func (b Builder) WithNamespace(namespace string) Builder {
	b.ApmServer.ObjectMeta.Namespace = namespace
	ref := b.ApmServer.Spec.Output.Elasticsearch.ElasticsearchRef
	if ref == nil {
		ref = &common.ObjectSelector{}
	}
	ref.Namespace = namespace
	b.ApmServer.Spec.Output.Elasticsearch.ElasticsearchRef = ref
	return b
}

func (b Builder) WithVersion(version string) Builder {
	b.ApmServer.Spec.Version = version
	return b
}

func (b Builder) WithNodeCount(count int) Builder {
	b.ApmServer.Spec.NodeCount = 1
	return b
}

// -- Helper functions

func (b Builder) RuntimeObjects() []runtime.Object {
	return []runtime.Object{&b.ApmServer}
}

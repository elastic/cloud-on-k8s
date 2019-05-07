// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apm

import (
	apmtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/apm/v1alpha1"
	assoctype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/associations/v1alpha1"
	common "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
)

var DefaultResources = common.ResourcesSpec{
	Limits: map[corev1.ResourceName]resource.Quantity{
		"memory": resource.MustParse("1G"),
	},
}

// -- Stack

type Builder struct {
	ApmServer   apmtype.ApmServer
	Association assoctype.ApmServerElasticsearchAssociation
}

func (b Builder) WithNamespace(namespace string) Builder {
	b.ApmServer.ObjectMeta.Namespace = namespace
	b.Association.ObjectMeta.Namespace = namespace
	b.Association.Spec.Elasticsearch.Namespace = namespace
	b.Association.Spec.ApmServer.Namespace = namespace
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
	return []runtime.Object{&b.ApmServer, &b.Association}
}

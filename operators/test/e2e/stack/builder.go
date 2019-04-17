// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stack

import (
	"github.com/elastic/k8s-operators/operators/pkg/apis/associations/v1alpha1"
	common "github.com/elastic/k8s-operators/operators/pkg/apis/common/v1alpha1"
	commonv1alpha1 "github.com/elastic/k8s-operators/operators/pkg/apis/common/v1alpha1"
	estype "github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	kbtype "github.com/elastic/k8s-operators/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/k8s-operators/operators/test/e2e/helpers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const defaultVersion = "6.7.0"

var DefaultResources = common.ResourcesSpec{
	Limits: map[corev1.ResourceName]resource.Quantity{
		corev1.ResourceMemory: resource.MustParse("1G"),
		corev1.ResourceCPU:    resource.MustParse("500m"),
	},
}

// -- Stack

type Builder struct {
	Elasticsearch estype.Elasticsearch
	Kibana        kbtype.Kibana
	Association   v1alpha1.KibanaElasticsearchAssociation
}

func NewStackBuilder(name string) Builder {
	meta := metav1.ObjectMeta{
		Name:      name,
		Namespace: helpers.DefaultNamespace,
	}
	selector := v1alpha1.ObjectSelector{
		Name:      name,
		Namespace: helpers.DefaultNamespace,
	}

	return Builder{
		Elasticsearch: estype.Elasticsearch{
			ObjectMeta: meta,
			Spec: estype.ElasticsearchSpec{
				Version: defaultVersion,
			},
		},
		Kibana: kbtype.Kibana{
			ObjectMeta: meta,
			Spec: kbtype.KibanaSpec{
				Version: defaultVersion,
			},
		},
		Association: v1alpha1.KibanaElasticsearchAssociation{
			ObjectMeta: meta,
			Spec: v1alpha1.KibanaElasticsearchAssociationSpec{
				Elasticsearch: selector,
				Kibana:        selector,
			},
		},
	}
}

func (b Builder) WithNamespace(namespace string) Builder {
	b.Elasticsearch.ObjectMeta.Namespace = namespace
	b.Kibana.ObjectMeta.Namespace = namespace
	b.Association.Namespace = namespace
	b.Association.Spec.Kibana.Namespace = namespace
	b.Association.Spec.Elasticsearch.Namespace = namespace
	return b
}

func (b Builder) WithVersion(version string) Builder {
	b.Elasticsearch.Spec.Version = version
	b.Kibana.Spec.Version = version
	return b
}

// -- ES Topology

func (b Builder) WithNoESTopology() Builder {
	b.Elasticsearch.Spec.Topology = []estype.TopologyElementSpec{}
	return b
}

func (b Builder) WithESMasterNodes(count int, resources common.ResourcesSpec) Builder {
	return b.withESTopologyElement(estype.TopologyElementSpec{
		NodeCount: int32(count),
		NodeTypes: estype.NodeTypesSpec{Master: true},
		Resources: resources,
	})
}

func (b Builder) WithESDataNodes(count int, resources common.ResourcesSpec) Builder {
	return b.withESTopologyElement(estype.TopologyElementSpec{
		NodeCount: int32(count),
		NodeTypes: estype.NodeTypesSpec{Data: true},
		Resources: resources,
	})
}

func (b Builder) WithESMasterDataNodes(count int, resources common.ResourcesSpec) Builder {
	return b.withESTopologyElement(estype.TopologyElementSpec{
		NodeCount: int32(count),
		NodeTypes: estype.NodeTypesSpec{Master: true, Data: true},
		Resources: resources,
	})
}

func (b Builder) withESTopologyElement(topologyElement estype.TopologyElementSpec) Builder {
	b.Elasticsearch.Spec.Topology = append(b.Elasticsearch.Spec.Topology, topologyElement)
	return b
}

func (b Builder) WithSecureSettings(secretName string) Builder {
	b.Elasticsearch.Spec.SecureSettings = &commonv1alpha1.ResourceNameReference{
		Name: secretName,
	}
	return b
}

// -- Kibana

func (b Builder) WithKibana(count int) Builder {
	b.Kibana.Spec.NodeCount = int32(count)
	return b
}

// -- Helper functions

func (b Builder) RuntimeObjects() []runtime.Object {
	return []runtime.Object{&b.Elasticsearch, &b.Kibana, &b.Association}
}

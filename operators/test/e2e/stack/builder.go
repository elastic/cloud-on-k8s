// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stack

import (
	common "github.com/elastic/k8s-operators/operators/pkg/apis/common/v1alpha1"
	stacktype "github.com/elastic/k8s-operators/operators/pkg/apis/deployments/v1alpha1"
	estype "github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	kbtype "github.com/elastic/k8s-operators/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/k8s-operators/operators/test/e2e/helpers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const defaultVersion = "6.4.2"

var DefaultResources = common.ResourcesSpec{
	Limits: map[corev1.ResourceName]resource.Quantity{
		"memory": resource.MustParse("1G"),
	},
}

// -- Stack

type Builder struct {
	stacktype.Stack
}

func NewStackBuilder(name string) Builder {
	return Builder{
		stacktype.Stack{
			ObjectMeta: v1.ObjectMeta{
				Name:      name,
				Namespace: helpers.DefaultNamespace,
			},
			Spec: stacktype.StackSpec{
				Elasticsearch: estype.ElasticsearchSpec{
					SetVMMaxMapCount: true,
				},
				Kibana:  kbtype.KibanaSpec{},
				Version: defaultVersion,
			},
		},
	}
}

func (b Builder) WithNamespace(namespace string) Builder {
	b.ObjectMeta.Namespace = namespace
	return b
}

func (b Builder) WithVersion(version string) Builder {
	b.Spec.Version = version
	return b
}

// -- ES Topologies

func (b Builder) WithNoESTopologies() Builder {
	b.Spec.Elasticsearch.Topologies = []estype.ElasticsearchTopologySpec{}
	return b
}

func (b Builder) WithESMasterNodes(count int, resources common.ResourcesSpec) Builder {
	return b.WithESTopology(estype.ElasticsearchTopologySpec{
		NodeCount: int32(count),
		NodeTypes: estype.NodeTypesSpec{Master: true},
		Resources: resources,
	})
}

func (b Builder) WithESDataNodes(count int, resources common.ResourcesSpec) Builder {
	return b.WithESTopology(estype.ElasticsearchTopologySpec{
		NodeCount: int32(count),
		NodeTypes: estype.NodeTypesSpec{Data: true},
		Resources: resources,
	})
}

func (b Builder) WithESMasterDataNodes(count int, resources common.ResourcesSpec) Builder {
	return b.WithESTopology(estype.ElasticsearchTopologySpec{
		NodeCount: int32(count),
		NodeTypes: estype.NodeTypesSpec{Master: true, Data: true},
		Resources: resources,
	})
}

func (b Builder) WithESTopology(topology estype.ElasticsearchTopologySpec) Builder {
	b.Spec.Elasticsearch.Topologies = append(b.Spec.Elasticsearch.Topologies, topology)
	return b
}

// -- Kibana

func (b Builder) WithKibana(count int) Builder {
	b.Spec.Kibana.NodeCount = 1
	return b
}

// -- Helper functions

func GetNamespacedName(stack stacktype.Stack) types.NamespacedName {
	return types.NamespacedName{
		Name:      stack.GetName(),
		Namespace: stack.GetNamespace(),
	}
}

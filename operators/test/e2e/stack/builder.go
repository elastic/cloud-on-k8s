// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stack

import (
	"github.com/elastic/k8s-operators/operators/pkg/apis/associations/v1alpha1"
	common "github.com/elastic/k8s-operators/operators/pkg/apis/common/v1alpha1"
	estype "github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	kbtype "github.com/elastic/k8s-operators/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/k8s-operators/operators/test/e2e/helpers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
	Elasticsearch estype.ElasticsearchCluster
	Kibana        kbtype.Kibana
	Association   v1alpha1.KibanaElasticsearchAssociation
}

func NewStackBuilder(name string) Builder {
	meta := v1.ObjectMeta{
		Name:      name,
		Namespace: helpers.DefaultNamespace,
	}
	selector := v1alpha1.ObjectSelector{
		Name:      name,
		Namespace: helpers.DefaultNamespace,
	}

	return Builder{
		Elasticsearch: estype.ElasticsearchCluster{
			ObjectMeta: meta,
			Spec: estype.ElasticsearchSpec{
				SetVMMaxMapCount: true,
				Version:          defaultVersion,
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

// -- ES Topologies

func (b Builder) WithNoESTopologies() Builder {
	b.Elasticsearch.Spec.Topologies = []estype.ElasticsearchTopologySpec{}
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
	b.Elasticsearch.Spec.Topologies = append(b.Elasticsearch.Spec.Topologies, topology)
	return b
}

// -- Kibana

func (b Builder) WithKibana(count int) Builder {
	b.Kibana.Spec.NodeCount = 1
	return b
}

// -- Helper functions

func (b Builder) RuntimeObjects() []runtime.Object {
	return []runtime.Object{&b.Elasticsearch, &b.Kibana, &b.Association}
}

func GetNamespacedName(stack Builder) types.NamespacedName {
	return types.NamespacedName{
		Name:      stack.Elasticsearch.Name,
		Namespace: stack.Elasticsearch.Namespace,
	}
}

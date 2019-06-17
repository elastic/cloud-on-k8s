// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stack

import (
	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	estype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	kbtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/params"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var DefaultResources = corev1.ResourceRequirements{
	Limits: map[corev1.ResourceName]resource.Quantity{
		corev1.ResourceMemory: resource.MustParse("2Gi"),
		corev1.ResourceCPU:    resource.MustParse("2"),
	},
}

func ESPodTemplate(resources corev1.ResourceRequirements) corev1.PodTemplateSpec {
	return corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:      v1alpha1.ElasticsearchContainerName,
					Resources: resources,
				},
			},
		},
	}
}

// -- Stack

type Builder struct {
	Elasticsearch estype.Elasticsearch
	Kibana        kbtype.Kibana
}

func NewStackBuilder(name string) Builder {
	meta := metav1.ObjectMeta{
		Name:      name,
		Namespace: params.Namespace,
	}

	return Builder{
		Elasticsearch: estype.Elasticsearch{
			ObjectMeta: meta,
			Spec: estype.ElasticsearchSpec{
				Version: params.ElasticStackVersion,
			},
		},
		Kibana: kbtype.Kibana{
			ObjectMeta: meta,
			Spec: kbtype.KibanaSpec{
				Version: params.ElasticStackVersion,
				ElasticsearchRef: commonv1alpha1.ObjectSelector{
					Name:      name,
					Namespace: params.Namespace,
				},
			},
		},
	}
}

func (b Builder) WithNamespace(namespace string) Builder {
	b.Elasticsearch.ObjectMeta.Namespace = namespace
	b.Kibana.ObjectMeta.Namespace = namespace
	b.Kibana.Spec.ElasticsearchRef.Namespace = namespace
	return b
}

func (b Builder) WithVersion(version string) Builder {
	b.Elasticsearch.Spec.Version = version
	b.Kibana.Spec.Version = version
	return b
}

// -- ES Nodes

func (b Builder) WithNoESTopology() Builder {
	b.Elasticsearch.Spec.Nodes = []estype.NodeSpec{}
	return b
}

func (b Builder) WithESMasterNodes(count int, resources corev1.ResourceRequirements) Builder {
	return b.withESTopologyElement(estype.NodeSpec{
		NodeCount: int32(count),
		Config: &commonv1alpha1.Config{
			Data: map[string]interface{}{
				estype.NodeData: "false",
			},
		},
		PodTemplate: ESPodTemplate(resources),
	})
}

func (b Builder) WithESDataNodes(count int, resources corev1.ResourceRequirements) Builder {
	return b.withESTopologyElement(estype.NodeSpec{
		NodeCount: int32(count),
		Config: &commonv1alpha1.Config{
			Data: map[string]interface{}{
				estype.NodeMaster: "false",
			},
		},
		PodTemplate: ESPodTemplate(resources),
	})
}

func (b Builder) WithESMasterDataNodes(count int, resources corev1.ResourceRequirements) Builder {
	return b.withESTopologyElement(estype.NodeSpec{
		NodeCount: int32(count),
		Config: &commonv1alpha1.Config{
			Data: map[string]interface{}{},
		},
		PodTemplate: ESPodTemplate(resources),
	})
}

func (b Builder) withESTopologyElement(topologyElement estype.NodeSpec) Builder {
	b.Elasticsearch.Spec.Nodes = append(b.Elasticsearch.Spec.Nodes, topologyElement)
	return b
}

func (b Builder) WithESSecureSettings(secretName string) Builder {
	b.Elasticsearch.Spec.SecureSettings = &commonv1alpha1.SecretRef{
		SecretName: secretName,
	}
	return b
}

// -- Kibana

func (b Builder) WithKibana(count int) Builder {
	b.Kibana.Spec.NodeCount = int32(count)
	return b
}

func (b Builder) WithKibanaSecureSettings(secretName string) Builder {
	b.Kibana.Spec.SecureSettings = &commonv1alpha1.SecretRef{
		SecretName: secretName,
	}
	return b
}

// -- Helper functions

func (b Builder) RuntimeObjects() []runtime.Object {
	return []runtime.Object{&b.Elasticsearch, &b.Kibana}
}

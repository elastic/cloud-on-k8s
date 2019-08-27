// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	commonv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	estype "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/rand"
)

func ESPodTemplate(resources corev1.ResourceRequirements) corev1.PodTemplateSpec {
	return corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			SecurityContext: test.DefaultSecurityContext(),
			Containers: []corev1.Container{
				{
					Name:      v1alpha1.ElasticsearchContainerName,
					Resources: resources,
				},
			},
		},
	}
}

// Builder to create Elasticsearch clusters
type Builder struct {
	Elasticsearch estype.Elasticsearch
	MutatedFrom   *Builder
}

var _ test.Builder = Builder{}

func NewBuilder(name string) Builder {
	meta := metav1.ObjectMeta{
		Name:      name,
		Namespace: test.Ctx().ManagedNamespace(0),
	}
	return Builder{
		Elasticsearch: estype.Elasticsearch{
			ObjectMeta: meta,
			Spec: estype.ElasticsearchSpec{
				SetVMMaxMapCount: test.BoolPtr(false),
				Version:          test.Ctx().ElasticStackVersion,
			},
		},
	}.WithSuffix(rand.String(4))
}

func (b Builder) WithSuffix(suffix string) Builder {
	b.Elasticsearch.ObjectMeta.Name = b.Elasticsearch.ObjectMeta.Name + "-" + suffix
	return b
}

func (b Builder) Ref() commonv1alpha1.ObjectSelector {
	return commonv1alpha1.ObjectSelector{
		Name:      b.Elasticsearch.Name,
		Namespace: b.Elasticsearch.Namespace,
	}
}

// WithRestrictedSecurityContext helps to enforce a restricted security context on the objects.
func (b Builder) WithRestrictedSecurityContext() Builder {
	b.Elasticsearch.Spec.SetVMMaxMapCount = test.BoolPtr(false)
	for idx := range b.Elasticsearch.Spec.Nodes {
		node := &b.Elasticsearch.Spec.Nodes[idx]
		node.PodTemplate.Spec.SecurityContext = test.DefaultSecurityContext()
	}
	return b
}

func (b Builder) WithNamespace(namespace string) Builder {
	b.Elasticsearch.ObjectMeta.Namespace = namespace
	return b
}

func (b Builder) WithVersion(version string) Builder {
	b.Elasticsearch.Spec.Version = version
	return b
}

func (b Builder) WithHTTPLoadBalancer() Builder {
	b.Elasticsearch.Spec.HTTP.Service.Spec.Type = corev1.ServiceTypeLoadBalancer
	return b
}

func (b Builder) WithHTTPSAN(ip string) Builder {
	b.Elasticsearch.Spec.HTTP.TLS.SelfSignedCertificate = &commonv1alpha1.SelfSignedCertificate{
		SubjectAlternativeNames: []commonv1alpha1.SubjectAlternativeName{{IP: ip}},
	}
	return b
}

// -- ES Nodes

func (b Builder) WithNoESTopology() Builder {
	b.Elasticsearch.Spec.Nodes = []estype.NodeSpec{}
	return b
}

func (b Builder) WithESMasterNodes(count int, resources corev1.ResourceRequirements) Builder {
	return b.WithNodeSpec(estype.NodeSpec{
		Name:      "master",
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
	return b.WithNodeSpec(estype.NodeSpec{
		Name:      "data",
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
	return b.WithNodeSpec(estype.NodeSpec{
		Name:      "masterdata",
		NodeCount: int32(count),
		Config: &commonv1alpha1.Config{
			Data: map[string]interface{}{},
		},
		PodTemplate: ESPodTemplate(resources),
	})
}

func (b Builder) WithNodeSpec(nodeSpec estype.NodeSpec) Builder {
	b.Elasticsearch.Spec.Nodes = append(b.Elasticsearch.Spec.Nodes, nodeSpec)
	return b
}

func (b Builder) WithESSecureSettings(secretNames ...string) Builder {
	refs := make([]commonv1alpha1.SecretRef, 0, len(secretNames))
	for i := range secretNames {
		refs = append(refs, commonv1alpha1.SecretRef{SecretName: secretNames[i]})
	}
	b.Elasticsearch.Spec.SecureSettings = refs
	return b
}

func (b Builder) WithEmptyDirVolumes() Builder {
	for i := range b.Elasticsearch.Spec.Nodes {
		b.Elasticsearch.Spec.Nodes[i].PodTemplate.Spec.Volumes = []corev1.Volume{
			{
				Name: volume.ElasticsearchDataVolumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
		}
	}
	return b
}

func (b Builder) WithPersistentVolumes(volumeName string) Builder {
	for i := range b.Elasticsearch.Spec.Nodes {
		name := volumeName
		b.Elasticsearch.Spec.Nodes[i].VolumeClaimTemplates = append(b.Elasticsearch.Spec.Nodes[i].VolumeClaimTemplates,
			corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
				},
			})
		b.Elasticsearch.Spec.Nodes[i].PodTemplate.Spec.Volumes = []corev1.Volume{
			{
				Name: name,
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: name,
						ReadOnly:  false,
					},
				},
			},
		}
	}
	return b
}

func (b Builder) WithPodTemplate(pt corev1.PodTemplateSpec) Builder {
	for i := range b.Elasticsearch.Spec.Nodes {
		b.Elasticsearch.Spec.Nodes[i].PodTemplate = pt
	}
	return b
}

func (b Builder) WithAdditionalConfig(nodeSpecCfg map[string]map[string]interface{}) Builder {
	var newNodes []estype.NodeSpec
	for node, cfg := range nodeSpecCfg {
		for _, n := range b.Elasticsearch.Spec.Nodes {
			if n.Name == node {
				newCfg := n.Config.DeepCopy()
				for k, v := range cfg {
					newCfg.Data[k] = v
				}
				n.Config = newCfg
			}
			newNodes = append(newNodes, n)
		}
	}
	b.Elasticsearch.Spec.Nodes = newNodes
	return b
}

// -- Helper functions

func (b Builder) RuntimeObjects() []runtime.Object {
	return []runtime.Object{&b.Elasticsearch}
}

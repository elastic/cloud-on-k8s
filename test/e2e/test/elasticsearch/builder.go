// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"reflect"

	"github.com/elastic/cloud-on-k8s/test/e2e/cmd/run"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/rand"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

const (
	// we setup our own storageClass with "volumeBindingMode: waitForFirstConsumer" that we
	// reference in the VolumeClaimTemplates section of the Elasticsearch spec
	defaultStorageClass = "e2e-default"
)

func ESPodTemplate(resources corev1.ResourceRequirements) corev1.PodTemplateSpec {
	return corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			SecurityContext: test.DefaultSecurityContext(),
			Containers: []corev1.Container{
				{
					Name:      esv1.ElasticsearchContainerName,
					Resources: resources,
				},
			},
		},
	}
}

// Builder to create Elasticsearch clusters
type Builder struct {
	Elasticsearch esv1.Elasticsearch
	MutatedFrom   *Builder
}

var _ test.Builder = Builder{}

func NewBuilder(name string) Builder {
	return newBuilder(name, rand.String(4))
}

func NewBuilderWithoutSuffix(name string) Builder {
	return newBuilder(name, "")
}

func newBuilder(name, randSuffix string) Builder {
	meta := metav1.ObjectMeta{
		Name:      name,
		Namespace: test.Ctx().ManagedNamespace(0),
		Labels:    map[string]string{run.TestNameLabel: name},
	}

	return Builder{
		Elasticsearch: esv1.Elasticsearch{
			ObjectMeta: meta,
			Spec: esv1.ElasticsearchSpec{
				Version: test.Ctx().ElasticStackVersion,
			},
		},
	}.
		WithSuffix(randSuffix).
		WithLabel(run.TestNameLabel, name)
}

func (b Builder) WithSuffix(suffix string) Builder {
	if suffix != "" {
		b.Elasticsearch.ObjectMeta.Name = b.Elasticsearch.ObjectMeta.Name + "-" + suffix
	}
	return b
}

func (b Builder) Ref() commonv1.ObjectSelector {
	return commonv1.ObjectSelector{
		Name:      b.Elasticsearch.Name,
		Namespace: b.Elasticsearch.Namespace,
	}
}

// WithRestrictedSecurityContext helps to enforce a restricted security context on the objects.
func (b Builder) WithRestrictedSecurityContext() Builder {
	for idx := range b.Elasticsearch.Spec.NodeSets {
		node := &b.Elasticsearch.Spec.NodeSets[idx]
		node.PodTemplate.Spec.SecurityContext = test.DefaultSecurityContext()
	}
	return b
}

func (b Builder) WithRemoteCluster(remoteEs Builder) Builder {
	b.Elasticsearch.Spec.RemoteClusters =
		append(b.Elasticsearch.Spec.RemoteClusters,
			esv1.RemoteCluster{
				Name:             remoteEs.Ref().Name,
				ElasticsearchRef: remoteEs.Ref(),
			})
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

func (b Builder) WithTLSDisabled(disabled bool) Builder {
	if b.Elasticsearch.Spec.HTTP.TLS.SelfSignedCertificate == nil {
		b.Elasticsearch.Spec.HTTP.TLS.SelfSignedCertificate = &commonv1.SelfSignedCertificate{}
	} else {
		b.Elasticsearch.Spec.HTTP.TLS.SelfSignedCertificate = b.Elasticsearch.Spec.HTTP.TLS.SelfSignedCertificate.DeepCopy()
	}
	b.Elasticsearch.Spec.HTTP.TLS.SelfSignedCertificate.Disabled = disabled
	return b
}

func (b Builder) WithHTTPSAN(ip string) Builder {
	b.Elasticsearch.Spec.HTTP.TLS.SelfSignedCertificate = &commonv1.SelfSignedCertificate{
		SubjectAlternativeNames: []commonv1.SubjectAlternativeName{{IP: ip}},
	}
	return b
}

// -- ES Nodes

func (b Builder) WithNoESTopology() Builder {
	b.Elasticsearch.Spec.NodeSets = []esv1.NodeSet{}
	return b
}

func (b Builder) WithESMasterNodes(count int, resources corev1.ResourceRequirements) Builder {
	return b.WithNodeSet(esv1.NodeSet{
		Name:  "master",
		Count: int32(count),
		Config: &commonv1.Config{
			Data: map[string]interface{}{
				esv1.NodeData: "false",
			},
		},
		PodTemplate: ESPodTemplate(resources),
	})
}

func (b Builder) WithESDataNodes(count int, resources corev1.ResourceRequirements) Builder {
	return b.WithNodeSet(esv1.NodeSet{
		Name:  "data",
		Count: int32(count),
		Config: &commonv1.Config{
			Data: map[string]interface{}{
				esv1.NodeMaster: "false",
			},
		},
		PodTemplate: ESPodTemplate(resources),
	})
}

func (b Builder) WithNamedESDataNodes(count int, name string, resources corev1.ResourceRequirements) Builder {
	return b.WithNodeSet(esv1.NodeSet{
		Name:  name,
		Count: int32(count),
		Config: &commonv1.Config{
			Data: map[string]interface{}{
				esv1.NodeMaster: "false",
			},
		},
		PodTemplate: ESPodTemplate(resources),
	})
}

func (b Builder) WithESMasterDataNodes(count int, resources corev1.ResourceRequirements) Builder {
	return b.WithNodeSet(esv1.NodeSet{
		Name:        "masterdata",
		Count:       int32(count),
		PodTemplate: ESPodTemplate(resources),
	})
}

func (b Builder) WithNodeSet(nodeSet esv1.NodeSet) Builder {
	// Make sure the config specifies "node.store.allow_mmap: false".
	// We disable mmap to avoid having to set the vm.max_map_count sysctl on test k8s nodes.
	if nodeSet.Config == nil {
		nodeSet.Config = &commonv1.Config{Data: map[string]interface{}{}}
	}
	nodeSet.Config.Data["node.store.allow_mmap"] = false

	// Propagates test-name label from top level resource.
	// Can be removed when https://github.com/elastic/cloud-on-k8s/issues/2652 is implemented.
	if nodeSet.PodTemplate.Labels == nil {
		nodeSet.PodTemplate.Labels = map[string]string{}
	}
	nodeSet.PodTemplate.Labels[run.TestNameLabel] = b.Elasticsearch.Labels[run.TestNameLabel]

	b.Elasticsearch.Spec.NodeSets = append(b.Elasticsearch.Spec.NodeSets, nodeSet)
	return b.WithDefaultPersistentVolumes()
}

func (b Builder) WithESSecureSettings(secretNames ...string) Builder {
	refs := make([]commonv1.SecretSource, 0, len(secretNames))
	for i := range secretNames {
		refs = append(refs, commonv1.SecretSource{SecretName: secretNames[i]})
	}
	b.Elasticsearch.Spec.SecureSettings = refs
	return b
}

func (b Builder) WithEmptyDirVolumes() Builder {
	for i := range b.Elasticsearch.Spec.NodeSets {
		// remove any default claim
		b.Elasticsearch.Spec.NodeSets[i].VolumeClaimTemplates = nil
		// setup an EmptyDir for the data volume
		b.Elasticsearch.Spec.NodeSets[i].PodTemplate.Spec.Volumes = []corev1.Volume{
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

func (b Builder) WithDefaultPersistentVolumes() Builder {
	storageClass := defaultStorageClass
	for i := range b.Elasticsearch.Spec.NodeSets {
		for _, existing := range b.Elasticsearch.Spec.NodeSets[i].VolumeClaimTemplates {
			if existing.Name == volume.ElasticsearchDataVolumeName {
				// already defined, don't set our defaults
				goto next
			}
		}

		// setup default claim with the custom storage class
		b.Elasticsearch.Spec.NodeSets[i].VolumeClaimTemplates = append(b.Elasticsearch.Spec.NodeSets[i].VolumeClaimTemplates,
			corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: volume.ElasticsearchDataVolumeName,
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
					StorageClassName: &storageClass,
				},
			})

	next:
	}
	return b
}

func (b Builder) WithPodTemplate(pt corev1.PodTemplateSpec) Builder {
	if pt.Labels == nil {
		pt.Labels = make(map[string]string)
	}
	pt.Labels[run.TestNameLabel] = b.Elasticsearch.Labels[run.TestNameLabel]
	for i := range b.Elasticsearch.Spec.NodeSets {
		b.Elasticsearch.Spec.NodeSets[i].PodTemplate = pt
	}
	return b
}

func (b Builder) WithAdditionalConfig(nodeSetCfg map[string]map[string]interface{}) Builder {
	var newNodeSets []esv1.NodeSet
	for nodeSetName, cfg := range nodeSetCfg {
		for _, n := range b.Elasticsearch.Spec.NodeSets {
			if n.Name == nodeSetName {
				newCfg := n.Config.DeepCopy()
				for k, v := range cfg {
					newCfg.Data[k] = v
				}
				n.Config = newCfg
			}
			newNodeSets = append(newNodeSets, n)
		}
	}
	b.Elasticsearch.Spec.NodeSets = newNodeSets
	return b
}

func (b Builder) WithChangeBudget(maxSurge, maxUnavailable int32) Builder {
	b.Elasticsearch.Spec.UpdateStrategy.ChangeBudget = esv1.ChangeBudget{
		MaxSurge:       pointer.Int32(maxSurge),
		MaxUnavailable: pointer.Int32(maxUnavailable),
	}
	return b
}

func (b Builder) WithMutatedFrom(builder *Builder) Builder {
	b.MutatedFrom = builder
	return b
}

func (b Builder) WithEnvironmentVariable(name, value string) Builder {
	for i, nodeSet := range b.Elasticsearch.Spec.NodeSets {
		for j, container := range nodeSet.PodTemplate.Spec.Containers {
			container.Env = append(container.Env, corev1.EnvVar{Name: name, Value: value})
			b.Elasticsearch.Spec.NodeSets[i].PodTemplate.Spec.Containers[j].Env = container.Env
		}
	}
	return b
}

func (b Builder) WithLabel(key, value string) Builder {
	if b.Elasticsearch.Labels == nil {
		b.Elasticsearch.Labels = make(map[string]string)
	}
	b.Elasticsearch.Labels[key] = value

	return b
}

// WithPodLabel sets the label in pod templates across all node sets.
// All invocations can be removed when
// https://github.com/elastic/cloud-on-k8s/issues/2652 is implemented.
func (b Builder) WithPodLabel(key, value string) Builder {
	for i := range b.Elasticsearch.Spec.NodeSets {
		if b.Elasticsearch.Spec.NodeSets[i].PodTemplate.Labels == nil {
			b.Elasticsearch.Spec.NodeSets[i].PodTemplate.Labels = make(map[string]string)
		}
		b.Elasticsearch.Spec.NodeSets[i].PodTemplate.Labels[key] = value
	}
	return b
}

// -- Helper functions

func (b Builder) RuntimeObjects() []runtime.Object {
	return []runtime.Object{&b.Elasticsearch}
}

func (b Builder) TriggersRollingUpgrade() bool {
	if b.MutatedFrom == nil {
		return false
	}
	// attempt do detect a rolling upgrade scenario
	// Important: this only checks ES version and spec, other changes such as secure settings update
	// are tricky to capture and ignored here.
	isVersionUpgrade := b.MutatedFrom.Elasticsearch.Spec.Version != b.Elasticsearch.Spec.Version
	httpOptionsChange := !reflect.DeepEqual(b.MutatedFrom.Elasticsearch.Spec.HTTP, b.Elasticsearch.Spec.HTTP)
	for _, initialNs := range b.MutatedFrom.Elasticsearch.Spec.NodeSets {
		for _, mutatedNs := range b.Elasticsearch.Spec.NodeSets {
			if initialNs.Name == mutatedNs.Name &&
				(isVersionUpgrade || httpOptionsChange || !reflect.DeepEqual(initialNs, mutatedNs)) {
				// a rolling upgrade is scheduled for that NodeSpec
				return true
			}
		}
	}
	return false
}

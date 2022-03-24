// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
	"github.com/elastic/cloud-on-k8s/test/e2e/cmd/run"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

const (
	// we setup our own storageClass with "volumeBindingMode: waitForFirstConsumer" that we
	// reference in the VolumeClaimTemplates section of the Elasticsearch spec
	DefaultStorageClass = "e2e-default"
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

	MutatedFrom *Builder

	// expectedElasticsearch is used to compare the deployed resources with the expected ones. This is only to be used in
	// situations where the Elasticsearch resource is modified by an external mechanism, like the autoscaling controller.
	// In such a situation the actual resources may diverge from what was originally specified in the builder.
	expectedElasticsearch *esv1.Elasticsearch
}

func (b Builder) DeepCopy() *Builder {
	es := b.Elasticsearch.DeepCopy()
	builderCopy := Builder{
		Elasticsearch: *es,
	}
	if b.expectedElasticsearch != nil {
		builderCopy.expectedElasticsearch = b.expectedElasticsearch.DeepCopy()
	}
	if b.MutatedFrom != nil {
		builderCopy.MutatedFrom = b.MutatedFrom.DeepCopy()
	}
	return &builderCopy
}

func (b Builder) GetExpectedElasticsearch() esv1.Elasticsearch {
	if b.expectedElasticsearch != nil {
		return *b.expectedElasticsearch
	}
	return b.Elasticsearch
}

var _ test.Builder = Builder{}

func (b Builder) SkipTest() bool {
	return false
}

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
	def := test.Ctx().ImageDefinitionFor(esv1.Kind)
	return Builder{
		Elasticsearch: esv1.Elasticsearch{
			ObjectMeta: meta,
			Spec: esv1.ElasticsearchSpec{
				Version: def.Version,
			},
		},
	}.
		WithImage(def.Image).
		WithSuffix(randSuffix).
		WithLabel(run.TestNameLabel, name)
}

func (b Builder) WithImage(image string) Builder {
	b.Elasticsearch.Spec.Image = image
	return b
}

func (b Builder) WithAnnotation(key, value string) Builder {
	if b.Elasticsearch.ObjectMeta.Annotations == nil {
		b.Elasticsearch.ObjectMeta.Annotations = make(map[string]string)
	}
	b.Elasticsearch.ObjectMeta.Annotations[key] = value
	return b
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

func (b Builder) WithCustomTransportCA(name string) Builder {
	b.Elasticsearch.Spec.Transport.TLS.Certificate.SecretName = name
	return b
}

func (b Builder) WithCustomHTTPCerts(name string) Builder {
	b.Elasticsearch.Spec.HTTP.TLS.Certificate.SecretName = name
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
			Data: MasterRoleCfg(b.Elasticsearch.Spec.Version),
		},
		PodTemplate: ESPodTemplate(resources),
	})
}

func (b Builder) WithESDataNodes(count int, resources corev1.ResourceRequirements) Builder {
	return b.WithNodeSet(esv1.NodeSet{
		Name:  "data",
		Count: int32(count),
		Config: &commonv1.Config{
			Data: DataRoleCfg(b.Elasticsearch.Spec.Version),
		},
		PodTemplate: ESPodTemplate(resources),
	})
}

func (b Builder) WithNamedESDataNodes(count int, name string, resources corev1.ResourceRequirements) Builder {
	return b.WithNodeSet(esv1.NodeSet{
		Name:  name,
		Count: int32(count),
		Config: &commonv1.Config{
			Data: DataRoleCfg(b.Elasticsearch.Spec.Version),
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

func (b Builder) WithESCoordinatingNodes(count int, resources corev1.ResourceRequirements) Builder {
	cfg := map[string]interface{}{}
	v := version.MustParse(b.Elasticsearch.Spec.Version)

	if v.GTE(version.From(7, 9, 0)) {
		cfg[esv1.NodeRoles] = []string{}
	} else {
		cfg[esv1.NodeMaster] = false
		cfg[esv1.NodeData] = false
		cfg[esv1.NodeIngest] = false
		cfg[esv1.NodeML] = false

		if v.GTE(version.From(7, 3, 0)) {
			cfg[esv1.NodeVotingOnly] = false
		}

		if v.GTE(version.From(7, 7, 0)) {
			cfg[esv1.NodeTransform] = false
			cfg[esv1.NodeRemoteClusterClient] = false
		}
	}

	return b.WithNodeSet(esv1.NodeSet{
		Name:  "coordinating",
		Count: int32(count),
		Config: &commonv1.Config{
			Data: cfg,
		},
		PodTemplate: ESPodTemplate(resources),
	})
}

func (b Builder) WithExpectedNodeSets(nodeSets ...esv1.NodeSet) Builder {
	builderCopy := b.DeepCopy()
	for _, nodeSet := range nodeSets {
		builderCopy.WithNodeSet(nodeSet)
	}
	b.expectedElasticsearch = &builderCopy.Elasticsearch
	return b
}

func (b Builder) WithNodeSet(nodeSet esv1.NodeSet) Builder {
	// Make sure the config specifies "node.store.allow_mmap: false".
	// We disable mmap to avoid having to set the vm.max_map_count sysctl on test k8s nodes.
	if nodeSet.Config == nil {
		nodeSet.Config = &commonv1.Config{Data: map[string]interface{}{}}
	}
	nodeSet.Config.Data["node.store.allow_mmap"] = false
	// helpful to debug test failures with red cluster health
	nodeSet.Config.Data["logger.org.elasticsearch.cluster.service.MasterService"] = "trace"

	// Propagates test-name label from top level resource.
	// Can be removed when https://github.com/elastic/cloud-on-k8s/issues/2652 is implemented.
	if nodeSet.PodTemplate.Labels == nil {
		nodeSet.PodTemplate.Labels = map[string]string{}
	}
	nodeSet.PodTemplate.Labels[run.TestNameLabel] = b.Elasticsearch.Labels[run.TestNameLabel]

	// If a nodeSet with the same name already exists, remove it
	for i := range b.Elasticsearch.Spec.NodeSets {
		if b.Elasticsearch.Spec.NodeSets[i].Name == nodeSet.Name {
			b.Elasticsearch.Spec.NodeSets[i] = nodeSet
			return b.WithDefaultPersistentVolumes()
		}
	}

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

func (b Builder) WithVolumeClaimDeletePolicy(policy esv1.VolumeClaimDeletePolicy) Builder {
	b.Elasticsearch.Spec.VolumeClaimDeletePolicy = policy
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
	storageClass := DefaultStorageClass
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

func (b Builder) WithMonitoring(metricsESRef commonv1.ObjectSelector, logsESRef commonv1.ObjectSelector) Builder {
	b.Elasticsearch.Spec.Monitoring.Metrics.ElasticsearchRefs = []commonv1.ObjectSelector{metricsESRef}
	b.Elasticsearch.Spec.Monitoring.Logs.ElasticsearchRefs = []commonv1.ObjectSelector{logsESRef}
	return b
}

func (b Builder) GetMetricsIndexPattern() string {
	v := version.MustParse(test.Ctx().ElasticStackVersion)
	if v.GTE(version.MinFor(8, 0, 0)) {
		return fmt.Sprintf("metricbeat-%d.%d.%d*", v.Major, v.Minor, v.Patch)
	}
	return ".monitoring-es-*"
}

func (b Builder) Name() string {
	return b.Elasticsearch.Name
}

func (b Builder) Namespace() string {
	return b.Elasticsearch.Namespace
}

func (b Builder) GetLogsCluster() *types.NamespacedName {
	if len(b.Elasticsearch.Spec.Monitoring.Logs.ElasticsearchRefs) == 0 {
		return nil
	}
	logsCluster := b.Elasticsearch.Spec.Monitoring.Logs.ElasticsearchRefs[0].NamespacedName()
	return &logsCluster
}

func (b Builder) GetMetricsCluster() *types.NamespacedName {
	if len(b.Elasticsearch.Spec.Monitoring.Metrics.ElasticsearchRefs) == 0 {
		return nil
	}
	metricsCluster := b.Elasticsearch.Spec.Monitoring.Metrics.ElasticsearchRefs[0].NamespacedName()
	return &metricsCluster
}

// -- Helper functions

func (b Builder) RuntimeObjects() []client.Object {
	return []client.Object{&b.Elasticsearch}
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

func MixedRolesCfg(ver string) map[string]interface{} {
	return roleCfg(ver, []esv1.NodeRole{esv1.MasterRole, esv1.DataRole}, map[string]bool{
		esv1.NodeMaster: true,
		esv1.NodeData:   true,
	})
}

func DataRoleCfg(ver string) map[string]interface{} {
	return roleCfg(ver, []esv1.NodeRole{esv1.DataRole}, map[string]bool{
		esv1.NodeMaster: false,
		esv1.NodeData:   true,
	})
}

func MasterRoleCfg(ver string) map[string]interface{} {
	return roleCfg(ver, []esv1.NodeRole{esv1.MasterRole}, map[string]bool{
		esv1.NodeMaster: true,
		esv1.NodeData:   false,
	})
}

func roleCfg(ver string, post78roles []esv1.NodeRole, pre79roles map[string]bool) map[string]interface{} {
	v := version.MustParse(ver)

	cfg := map[string]interface{}{}
	if v.GTE(version.From(7, 9, 0)) {
		cfg[esv1.NodeRoles] = post78roles
	} else {
		for k, v := range pre79roles {
			cfg[k] = v
		}
	}

	return cfg
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/cmd/run"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

// Builder to create Kibana instances
type Builder struct {
	Kibana                   kbv1.Kibana
	ExternalElasticsearchRef commonv1.ObjectSelector
	MutatedFrom              *Builder
}

var _ test.Builder = Builder{}
var _ test.Subject = Builder{}

func (b Builder) SkipTest() bool {
	return false
}

func NewBuilder(name string) Builder {
	return newBuilder(name, rand.String(4))
}

func NewBuilderWithoutSuffix(name string) Builder {
	return newBuilder(name, "")
}

func (b Builder) Ref() commonv1.ObjectSelector {
	return commonv1.ObjectSelector{
		Name:      b.Kibana.Name,
		Namespace: b.Kibana.Namespace,
	}
}

func newBuilder(name, randSuffix string) Builder {
	meta := metav1.ObjectMeta{
		Name:      name,
		Namespace: test.Ctx().ManagedNamespace(0),
	}
	def := test.Ctx().ImageDefinitionFor(kbv1.Kind)
	return Builder{
		Kibana: kbv1.Kibana{
			ObjectMeta: meta,
			Spec: kbv1.KibanaSpec{
				Version: def.Version,
				PodTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						SecurityContext: test.DefaultSecurityContext(),
					},
				},
			},
		},
	}.
		WithImage(def.Image).
		WithSuffix(randSuffix).
		WithLabel(run.TestNameLabel, name).
		WithPodLabel(run.TestNameLabel, name)
}

func (b Builder) WithImage(image string) Builder {
	b.Kibana.Spec.Image = image
	return b
}

func (b Builder) WithSuffix(suffix string) Builder {
	if suffix != "" {
		b.Kibana.ObjectMeta.Name = b.Kibana.ObjectMeta.Name + "-" + suffix
	}
	return b
}

func (b Builder) WithElasticsearchRef(ref commonv1.ObjectSelector) Builder {
	b.Kibana.Spec.ElasticsearchRef = ref
	return b
}

func (b Builder) WithEnterpriseSearchRef(ref commonv1.ObjectSelector) Builder {
	b.Kibana.Spec.EnterpriseSearchRef = ref
	return b
}

func (b Builder) WithExternalElasticsearchRef(ref commonv1.ObjectSelector) Builder {
	b.ExternalElasticsearchRef = ref
	return b
}

// WithRestrictedSecurityContext helps to enforce a restricted security context on the objects.
func (b Builder) WithRestrictedSecurityContext() Builder {
	b.Kibana.Spec.PodTemplate.Spec.SecurityContext = test.DefaultSecurityContext()
	return b
}

func (b Builder) WithNamespace(namespace string) Builder {
	b.Kibana.ObjectMeta.Namespace = namespace
	return b
}

func (b Builder) WithVersion(version string) Builder {
	b.Kibana.Spec.Version = version
	return b
}

func (b Builder) WithNodeCount(count int) Builder {
	b.Kibana.Spec.Count = int32(count)
	return b
}

func (b Builder) WithKibanaSecureSettings(secretNames ...string) Builder {
	refs := make([]commonv1.SecretSource, 0, len(secretNames))
	for i := range secretNames {
		refs = append(refs, commonv1.SecretSource{SecretName: secretNames[i]})
	}
	b.Kibana.Spec.SecureSettings = refs
	return b
}

func (b Builder) WithResources(resources corev1.ResourceRequirements) Builder {
	if len(b.Kibana.Spec.PodTemplate.Spec.Containers) == 0 {
		b.Kibana.Spec.PodTemplate.Spec.Containers = []corev1.Container{
			{Name: kbv1.KibanaContainerName},
		}
	}
	for i, c := range b.Kibana.Spec.PodTemplate.Spec.Containers {
		if c.Name == kbv1.KibanaContainerName {
			c.Resources = resources
			b.Kibana.Spec.PodTemplate.Spec.Containers[i] = c
		}
	}
	return b
}

func (b Builder) WithMutatedFrom(mutatedFrom *Builder) Builder {
	b.MutatedFrom = mutatedFrom
	return b
}

func (b Builder) WithLabel(key, value string) Builder {
	if b.Kibana.Labels == nil {
		b.Kibana.Labels = make(map[string]string)
	}
	b.Kibana.Labels[key] = value

	return b
}

// WithPodLabel sets the label in the pod template. All invocations can be removed when
// https://github.com/elastic/cloud-on-k8s/issues/2652 is implemented.
func (b Builder) WithPodLabel(key, value string) Builder {
	labels := b.Kibana.Spec.PodTemplate.Labels
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[key] = value
	b.Kibana.Spec.PodTemplate.Labels = labels
	return b
}

func (b Builder) WithTLSDisabled(disabled bool) Builder {
	if b.Kibana.Spec.HTTP.TLS.SelfSignedCertificate == nil {
		b.Kibana.Spec.HTTP.TLS.SelfSignedCertificate = &commonv1.SelfSignedCertificate{}
	} else {
		b.Kibana.Spec.HTTP.TLS.SelfSignedCertificate = b.Kibana.Spec.HTTP.TLS.SelfSignedCertificate.DeepCopy()
	}
	b.Kibana.Spec.HTTP.TLS.SelfSignedCertificate.Disabled = disabled
	return b
}

// WithAPMIntegration adds configuration that makes Kibana install APM integration on start up. Starting with 8.0.0,
// index templates for APM Server are not installed by APM Server, but during APM integration installation in Kibana.
func (b Builder) WithAPMIntegration() Builder {
	if version.MustParse(b.Kibana.Spec.Version).LT(version.MinFor(8, 0, 0)) {
		// configuring APM integration is not necessary below 8.0.0, no-op
		return b
	}

	return b.WithConfig(map[string]interface{}{
		"xpack.fleet.packages": []map[string]interface{}{
			{
				"name":    "apm",
				"version": "latest",
			},
		},
	})
}

func (b Builder) WithConfig(config map[string]interface{}) Builder {
	b.Kibana.Spec.Config = &commonv1.Config{
		Data: config,
	}

	return b
}

func (b Builder) WithMonitoring(metricsESRef commonv1.ObjectSelector, logsESRef commonv1.ObjectSelector) Builder {
	b.Kibana.Spec.Monitoring.Metrics.ElasticsearchRefs = []commonv1.ObjectSelector{metricsESRef}
	b.Kibana.Spec.Monitoring.Logs.ElasticsearchRefs = []commonv1.ObjectSelector{logsESRef}
	return b
}

func (b Builder) GetMetricsIndexPattern() string {
	v := version.MustParse(test.Ctx().ElasticStackVersion)
	if v.GTE(version.MinFor(8, 0, 0)) {
		return fmt.Sprintf("metricbeat-%d.%d.%d*", v.Major, v.Minor, v.Patch)
	}

	return ".monitoring-kibana-*"
}

func (b Builder) Name() string {
	return b.Kibana.Name
}

func (b Builder) Namespace() string {
	return b.Kibana.Namespace
}

func (b Builder) GetLogsCluster() *types.NamespacedName {
	if len(b.Kibana.Spec.Monitoring.Logs.ElasticsearchRefs) == 0 {
		return nil
	}
	logsCluster := b.Kibana.Spec.Monitoring.Logs.ElasticsearchRefs[0].NamespacedName()
	return &logsCluster
}

func (b Builder) GetMetricsCluster() *types.NamespacedName {
	if len(b.Kibana.Spec.Monitoring.Metrics.ElasticsearchRefs) == 0 {
		return nil
	}
	metricsCluster := b.Kibana.Spec.Monitoring.Metrics.ElasticsearchRefs[0].NamespacedName()
	return &metricsCluster
}

// -- test.Subject impl

func (b Builder) NSN() types.NamespacedName {
	return k8s.ExtractNamespacedName(&b.Kibana)
}

func (b Builder) Kind() string {
	return kbv1.Kind
}

func (b Builder) Spec() interface{} {
	return b.Kibana.Spec
}

func (b Builder) Count() int32 {
	return b.Kibana.Spec.Count
}

func (b Builder) ServiceName() string {
	return b.Kibana.Name + "-kb-http"
}

func (b Builder) ListOptions() []client.ListOption {
	return test.KibanaPodListOptions(b.Kibana.Namespace, b.Kibana.Name)
}

// -- Helper functions

func (b Builder) RuntimeObjects() []client.Object {
	return []client.Object{&b.Kibana}
}

func (b Builder) ElasticsearchRef() commonv1.ObjectSelector {
	if b.ExternalElasticsearchRef.IsDefined() {
		return b.ExternalElasticsearchRef
	}
	// if no external Elasticsearch cluster is defined, use the ElasticsearchRef
	return b.Kibana.EsAssociation().AssociationRef()
}

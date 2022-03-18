// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package apmserver

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/cmd/run"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

// Builder to create APM Servers
type Builder struct {
	ApmServer apmv1.ApmServer
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

func newBuilder(name, randSuffix string) Builder {
	meta := metav1.ObjectMeta{
		Name:      name,
		Namespace: test.Ctx().ManagedNamespace(0),
	}
	def := test.Ctx().ImageDefinitionFor(apmv1.Kind)
	return Builder{
		ApmServer: apmv1.ApmServer{
			ObjectMeta: meta,
			Spec: apmv1.ApmServerSpec{
				Count:   1,
				Version: def.Version,
				Config: &commonv1.Config{
					Data: map[string]interface{}{
						"apm-server.ilm.enabled": false,
					},
				},
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

func (b Builder) WithSuffix(suffix string) Builder {
	if suffix != "" {
		b.ApmServer.ObjectMeta.Name = b.ApmServer.ObjectMeta.Name + "-" + suffix
	}
	return b
}

func (b Builder) WithImage(image string) Builder {
	b.ApmServer.Spec.Image = image
	return b
}

func (b Builder) WithRestrictedSecurityContext() Builder {
	b.ApmServer.Spec.PodTemplate.Spec.SecurityContext = test.DefaultSecurityContext()
	return b
}

func (b Builder) WithNamespace(namespace string) Builder {
	b.ApmServer.ObjectMeta.Namespace = namespace
	return b
}

func (b Builder) WithVersion(version string) Builder {
	b.ApmServer.Spec.Version = version
	return b
}

func (b Builder) WithNodeCount(count int) Builder {
	b.ApmServer.Spec.Count = int32(count)
	return b
}

func (b Builder) WithElasticsearchRef(ref commonv1.ObjectSelector) Builder {
	b.ApmServer.Spec.ElasticsearchRef = ref
	return b
}

func (b Builder) WithKibanaRef(ref commonv1.ObjectSelector) Builder {
	b.ApmServer.Spec.KibanaRef = ref
	return b
}

func (b Builder) WithConfig(cfg map[string]interface{}) Builder {
	if b.ApmServer.Spec.Config == nil || b.ApmServer.Spec.Config.Data == nil {
		b.ApmServer.Spec.Config = &commonv1.Config{
			Data: cfg,
		}
		return b
	}

	for k, v := range cfg {
		b.ApmServer.Spec.Config.Data[k] = v
	}
	return b
}

func (b Builder) WithRUM(enabled bool) Builder {
	return b.WithConfig(map[string]interface{}{"apm-server.rum.enabled": true})
}

func (b Builder) WithHTTPCfg(cfg commonv1.HTTPConfig) Builder {
	b.ApmServer.Spec.HTTP = cfg
	return b
}

func (b Builder) WithLabel(key, value string) Builder {
	if b.ApmServer.Labels == nil {
		b.ApmServer.Labels = make(map[string]string)
	}
	b.ApmServer.Labels[key] = value

	return b
}

// WithPodLabel sets the label in the pod template. All invocations can be removed when
// https://github.com/elastic/cloud-on-k8s/issues/2652 is implemented.
func (b Builder) WithPodLabel(key, value string) Builder {
	labels := b.ApmServer.Spec.PodTemplate.Labels
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[key] = value
	b.ApmServer.Spec.PodTemplate.Labels = labels
	return b
}

// WithoutIntegrationCheck adds APM Server configuration that prevents APM Server from checking for APM index templates.
// Starting with 8.0.0, these templates are installed by APM integration. As all integrations are installed through
// Kibana, when there is no Kibana in the deployment, the index templates are not present and our E2E tests checks
// would fail.
func (b Builder) WithoutIntegrationCheck() Builder {
	if version.MustParse(b.ApmServer.Spec.Version).LT(version.MinFor(8, 0, 0)) {
		// disabling integration check is not necessary below 8.0.0, no-op
		return b
	}

	return b.WithConfig(map[string]interface{}{
		"apm-server.data_streams.wait_for_integration": false,
	})
}

func (b Builder) NSN() types.NamespacedName {
	return k8s.ExtractNamespacedName(&b.ApmServer)
}

func (b Builder) Kind() string {
	return apmv1.Kind
}

func (b Builder) Spec() interface{} {
	return b.ApmServer.Spec
}

func (b Builder) Count() int32 {
	return b.ApmServer.Spec.Count
}

func (b Builder) ServiceName() string {
	return b.ApmServer.Name + "-apm-http"
}

func (b Builder) ListOptions() []client.ListOption {
	return test.ApmServerPodListOptions(b.ApmServer.Namespace, b.ApmServer.Name)
}

// -- Helper functions

func (b Builder) RuntimeObjects() []client.Object {
	return []client.Object{&b.ApmServer}
}

func (b Builder) RUMEnabled() bool {
	rumEnabledConfig, ok := b.ApmServer.Spec.Config.Data["apm-server.rum.enabled"]
	if ok {
		if v, ok := rumEnabledConfig.(bool); ok {
			return v
		}
	}
	return false // rum disabled by default
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package maps

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/apis/maps/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/cmd/run"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

type Builder struct {
	EMS         v1alpha1.ElasticMapsServer
	MutatedFrom *Builder
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
	def := test.Ctx().ImageDefinitionFor(v1alpha1.Kind)
	return Builder{
		EMS: v1alpha1.ElasticMapsServer{
			ObjectMeta: meta,
			Spec: v1alpha1.MapsSpec{
				Count:   1,
				Version: def.Version,
			},
		},
	}.
		WithImage(def.Image).
		WithSuffix(randSuffix).
		WithLabel(run.TestNameLabel, name).
		WithPodLabel(run.TestNameLabel, name)
}

func (b Builder) WithImage(image string) Builder {
	b.EMS.Spec.Image = image
	return b
}

func (b Builder) WithSuffix(suffix string) Builder {
	if suffix != "" {
		b.EMS.ObjectMeta.Name = b.EMS.ObjectMeta.Name + "-" + suffix
	}
	return b
}

func (b Builder) WithLabel(key, value string) Builder {
	if b.EMS.Labels == nil {
		b.EMS.Labels = make(map[string]string)
	}
	b.EMS.Labels[key] = value

	return b
}

// WithRestrictedSecurityContext helps to enforce a restricted security context on the objects.
func (b Builder) WithRestrictedSecurityContext() Builder {
	b.EMS.Spec.PodTemplate.Spec.SecurityContext = test.DefaultSecurityContext()
	return b
}

func (b Builder) WithElasticsearchRef(ref commonv1.ObjectSelector) Builder {
	b.EMS.Spec.ElasticsearchRef = ref
	return b
}

func (b Builder) WithNamespace(namespace string) Builder {
	b.EMS.ObjectMeta.Namespace = namespace
	return b
}

func (b Builder) WithVersion(version string) Builder {
	b.EMS.Spec.Version = version
	return b
}

func (b Builder) WithNodeCount(count int) Builder {
	b.EMS.Spec.Count = int32(count)
	return b
}

// WithPodLabel sets the label in the pod template. All invocations can be removed when
// https://github.com/elastic/cloud-on-k8s/issues/2652 is implemented.
func (b Builder) WithPodLabel(key, value string) Builder {
	labels := b.EMS.Spec.PodTemplate.Labels
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[key] = value
	b.EMS.Spec.PodTemplate.Labels = labels
	return b
}

func (b Builder) WithTLSDisabled(disabled bool) Builder {
	if b.EMS.Spec.HTTP.TLS.SelfSignedCertificate == nil {
		b.EMS.Spec.HTTP.TLS.SelfSignedCertificate = &commonv1.SelfSignedCertificate{}
	}
	b.EMS.Spec.HTTP.TLS.SelfSignedCertificate.Disabled = disabled
	return b
}

func (b Builder) WithMutatedFrom(mutatedFrom *Builder) Builder {
	b.MutatedFrom = mutatedFrom
	return b
}

// test.Subject impl

func (b Builder) NSN() types.NamespacedName {
	return k8s.ExtractNamespacedName(&b.EMS)
}

func (b Builder) Kind() string {
	return v1alpha1.Kind
}

func (b Builder) Spec() interface{} {
	return b.EMS.Spec
}

func (b Builder) Count() int32 {
	return b.EMS.Spec.Count
}

func (b Builder) ServiceName() string {
	return b.EMS.Name + "-ems-http"
}

func (b Builder) ListOptions() []client.ListOption {
	return test.MapsPodListOptions(b.EMS.Namespace, b.EMS.Name)
}

func (b Builder) MutationReversalTestContext() test.ReversalTestContext {
	panic("implement me")
}

func (b Builder) SkipTest() bool {
	// only execute EMS tests if we have a test license to work with
	if test.Ctx().TestLicense == "" {
		return true
	}
	ver := version.MustParse(b.EMS.Spec.Version)
	return version.SupportedMapsVersions.WithinRange(ver) != nil
}

var _ test.Builder = Builder{}

var _ test.Subject = Builder{}

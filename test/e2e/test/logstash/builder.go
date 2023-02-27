// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/cmd/run"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
)

type Builder struct {
	Logstash    v1alpha1.Logstash
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
		Logstash: v1alpha1.Logstash{
			ObjectMeta: meta,
			Spec: v1alpha1.LogstashSpec{
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
	b.Logstash.Spec.Image = image
	return b
}

func (b Builder) WithSuffix(suffix string) Builder {
	if suffix != "" {
		b.Logstash.ObjectMeta.Name = b.Logstash.ObjectMeta.Name + "-" + suffix
	}
	return b
}

func (b Builder) WithLabel(key, value string) Builder {
	if b.Logstash.Labels == nil {
		b.Logstash.Labels = make(map[string]string)
	}
	b.Logstash.Labels[key] = value

	return b
}

// WithRestrictedSecurityContext helps to enforce a restricted security context on the objects.
func (b Builder) WithRestrictedSecurityContext() Builder {
	b.Logstash.Spec.PodTemplate.Spec.SecurityContext = test.DefaultSecurityContext()
	return b
}

func (b Builder) WithNamespace(namespace string) Builder {
	b.Logstash.ObjectMeta.Namespace = namespace
	return b
}

func (b Builder) WithVersion(version string) Builder {
	b.Logstash.Spec.Version = version
	return b
}

func (b Builder) WithNodeCount(count int) Builder {
	b.Logstash.Spec.Count = int32(count)
	return b
}

// WithPodLabel sets the label in the pod template. All invocations can be removed when
// https://github.com/elastic/cloud-on-k8s/issues/2652 is implemented.
func (b Builder) WithPodLabel(key, value string) Builder {
	labels := b.Logstash.Spec.PodTemplate.Labels
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[key] = value
	b.Logstash.Spec.PodTemplate.Labels = labels
	return b
}

func (b Builder) WithMutatedFrom(mutatedFrom *Builder) Builder {
	b.MutatedFrom = mutatedFrom
	return b
}

func (b Builder) NSN() types.NamespacedName {
	return k8s.ExtractNamespacedName(&b.Logstash)
}

func (b Builder) Kind() string {
	return v1alpha1.Kind
}

func (b Builder) Spec() interface{} {
	return b.Logstash.Spec
}

func (b Builder) Count() int32 {
	return b.Logstash.Spec.Count
}

func (b Builder) ServiceName() string {
	return b.Logstash.Name + "-ls-default"
}

func (b Builder) ListOptions() []client.ListOption {
	return test.LogstashPodListOptions(b.Logstash.Namespace, b.Logstash.Name)
}

func (b Builder) SkipTest() bool {
	supportedVersions := version.SupportedLogstashVersions

	ver := version.MustParse(b.Logstash.Spec.Version)
	return supportedVersions.WithinRange(ver) != nil
}

var _ test.Builder = Builder{}
var _ test.Subject = Builder{}

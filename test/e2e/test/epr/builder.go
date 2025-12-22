// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package epr

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/apis/packageregistry/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/cmd/run"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
)

type Builder struct {
	EPR         v1alpha1.PackageRegistry
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
	return Builder{
		EPR: v1alpha1.PackageRegistry{
			ObjectMeta: meta,
			Spec: v1alpha1.PackageRegistrySpec{
				Count: 1,
				// replace with smaller image
				Image: "docker.elastic.co/package-registry/distribution:lite",
			},
		},
	}.
		WithVersion(test.Ctx().ElasticStackVersion).
		WithSuffix(randSuffix).
		WithLabel(run.TestNameLabel, name).
		WithPodLabel(run.TestNameLabel, name)
}

func (b Builder) WithSuffix(suffix string) Builder {
	if suffix != "" {
		b.EPR.ObjectMeta.Name = b.EPR.ObjectMeta.Name + "-" + suffix
	}
	return b
}

func (b Builder) WithLabel(key, value string) Builder {
	if b.EPR.Labels == nil {
		b.EPR.Labels = make(map[string]string)
	}
	b.EPR.Labels[key] = value

	return b
}

func (b Builder) WithAnnotation(key, value string) Builder {
	if b.EPR.Annotations == nil {
		b.EPR.Annotations = make(map[string]string)
	}
	b.EPR.Annotations[key] = value
	return b
}

// WithRestrictedSecurityContext helps to enforce a restricted security context on the objects.
func (b Builder) WithRestrictedSecurityContext() Builder {
	b.EPR.Spec.PodTemplate.Spec.SecurityContext = test.DefaultSecurityContext()
	return b
}

func (b Builder) WithNamespace(namespace string) Builder {
	b.EPR.ObjectMeta.Namespace = namespace
	return b
}

func (b Builder) Ref() commonv1.ObjectSelector {
	return commonv1.ObjectSelector{
		Name:      b.EPR.Name,
		Namespace: b.EPR.Namespace,
	}
}

func (b Builder) WithVersion(version string) Builder {
	b.EPR.Spec.Version = version
	return b
}

func (b Builder) WithNodeCount(count int) Builder {
	b.EPR.Spec.Count = int32(count)
	return b
}

// WithPodLabel sets the label in the pod template. All invocations can be removed when
// https://github.com/elastic/cloud-on-k8s/issues/2652 is implemented.
func (b Builder) WithPodLabel(key, value string) Builder {
	labels := b.EPR.Spec.PodTemplate.Labels
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[key] = value
	b.EPR.Spec.PodTemplate.Labels = labels
	return b
}

func (b Builder) WithTLSDisabled(disabled bool) Builder {
	if b.EPR.Spec.HTTP.TLS.SelfSignedCertificate == nil {
		b.EPR.Spec.HTTP.TLS.SelfSignedCertificate = &commonv1.SelfSignedCertificate{}
	}
	b.EPR.Spec.HTTP.TLS.SelfSignedCertificate.Disabled = disabled
	return b
}

func (b Builder) WithMutatedFrom(mutatedFrom *Builder) Builder {
	b.MutatedFrom = mutatedFrom
	return b
}

// test.Subject impl

func (b Builder) NSN() types.NamespacedName {
	return k8s.ExtractNamespacedName(&b.EPR)
}

func (b Builder) Kind() string {
	return v1alpha1.Kind
}

func (b Builder) Spec() interface{} {
	return b.EPR.Spec
}

func (b Builder) Count() int32 {
	return b.EPR.Spec.Count
}

func (b Builder) ServiceName() string {
	return b.EPR.Name + "-epr-http"
}

func (b Builder) ListOptions() []client.ListOption {
	return test.EPRPodListOptions(b.EPR.Namespace, b.EPR.Name)
}

func (b Builder) SkipTest() bool {
	// EPR doesn't require a license like Maps does
	ver := version.MustParse(b.EPR.Spec.Version)
	return version.SupportedPackageRegistryVersions.WithinRange(ver) != nil
}

var _ test.Builder = Builder{}

var _ test.Subject = Builder{}

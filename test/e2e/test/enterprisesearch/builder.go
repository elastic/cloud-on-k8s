// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package enterprisesearch

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	entv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/test/e2e/cmd/run"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

var (
	minVersion = version.MustParse("7.7.0")
	// Starting from 7.9.0, Enterprise Search is incompatible with Openshift default SCC due to file permission errors.
	// See https://github.com/elastic/cloud-on-k8s/issues/3656.
	ocpFirstIncompatibleVersion = version.MustParse("7.9.0")
)

// Builder to create Enterprise Search.
type Builder struct {
	EnterpriseSearch entv1beta1.EnterpriseSearch
	MutatedFrom      *Builder
}

var _ test.Builder = Builder{}

// SkipTest returns true if the version is not at least 7.7.0, or if the version is incompatible with Openshift.
func (b Builder) SkipTest() bool {
	v := version.MustParse(b.EnterpriseSearch.Spec.Version)
	// skip if not at least 7.0
	return !v.IsSameOrAfter(minVersion) ||
		// or if incompatible with Openshift
		isIncompatibleWithOcp(v)
}

func isIncompatibleWithOcp(v version.Version) bool {
	if !test.Ctx().OcpCluster {
		return false
	}
	if v.IsSameOrAfter(ocpFirstIncompatibleVersion) {
		return true
	}

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

	return Builder{
		EnterpriseSearch: entv1beta1.EnterpriseSearch{
			ObjectMeta: meta,
			Spec: entv1beta1.EnterpriseSearchSpec{
				Count:   1,
				Version: test.Ctx().ElasticStackVersion,
			},
		},
	}.
		WithSuffix(randSuffix).
		WithLabel(run.TestNameLabel, name).
		WithPodLabel(run.TestNameLabel, name)
}

func (b Builder) WithSuffix(suffix string) Builder {
	if suffix != "" {
		b.EnterpriseSearch.ObjectMeta.Name = b.EnterpriseSearch.ObjectMeta.Name + "-" + suffix
	}
	return b
}

// WithRestrictedSecurityContext helps to enforce a restricted security context on the objects.
func (b Builder) WithRestrictedSecurityContext() Builder {
	b.EnterpriseSearch.Spec.PodTemplate.Spec.SecurityContext = test.DefaultSecurityContext()
	return b
}

func (b Builder) WithElasticsearchRef(ref commonv1.ObjectSelector) Builder {
	b.EnterpriseSearch.Spec.ElasticsearchRef = ref
	return b
}

func (b Builder) WithNamespace(namespace string) Builder {
	b.EnterpriseSearch.ObjectMeta.Namespace = namespace
	return b
}

func (b Builder) WithVersion(version string) Builder {
	b.EnterpriseSearch.Spec.Version = version
	return b
}

func (b Builder) WithNodeCount(count int) Builder {
	b.EnterpriseSearch.Spec.Count = int32(count)
	return b
}

func (b Builder) WithTLSDisabled(disabled bool) Builder {
	if b.EnterpriseSearch.Spec.HTTP.TLS.SelfSignedCertificate == nil {
		b.EnterpriseSearch.Spec.HTTP.TLS.SelfSignedCertificate = &commonv1.SelfSignedCertificate{}
	}
	b.EnterpriseSearch.Spec.HTTP.TLS.SelfSignedCertificate.Disabled = disabled
	return b
}

func (b Builder) WithConfig(cfg map[string]interface{}) Builder {
	if b.EnterpriseSearch.Spec.Config == nil || b.EnterpriseSearch.Spec.Config.Data == nil {
		b.EnterpriseSearch.Spec.Config = &commonv1.Config{
			Data: cfg,
		}
		return b
	}
	for k, v := range cfg {
		b.EnterpriseSearch.Spec.Config.Data[k] = v
	}
	return b
}

func (b Builder) WithConfigRef(ref *commonv1.ConfigSource) Builder {
	b.EnterpriseSearch.Spec.ConfigRef = ref
	return b
}

func (b Builder) WithMutatedFrom(mutatedFrom *Builder) Builder {
	b.MutatedFrom = mutatedFrom
	return b
}

func (b Builder) WithLabel(key, value string) Builder {
	if b.EnterpriseSearch.Labels == nil {
		b.EnterpriseSearch.Labels = make(map[string]string)
	}
	b.EnterpriseSearch.Labels[key] = value

	return b
}

// WithPodLabel sets the label in the pod template. All invocations can be removed when
// https://github.com/elastic/cloud-on-k8s/issues/2652 is implemented.
func (b Builder) WithPodLabel(key, value string) Builder {
	labels := b.EnterpriseSearch.Spec.PodTemplate.Labels
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[key] = value
	b.EnterpriseSearch.Spec.PodTemplate.Labels = labels
	return b
}

// -- Helper functions

func (b Builder) RuntimeObjects() []client.Object {
	return []client.Object{&b.EnterpriseSearch}
}

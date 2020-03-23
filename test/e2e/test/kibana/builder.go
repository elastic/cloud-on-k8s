// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/rand"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

// Builder to create Kibana instances
type Builder struct {
	Kibana                   kbv1.Kibana
	ExternalElasticsearchRef commonv1.ObjectSelector
	MutatedFrom              *Builder
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
	}
	return Builder{
		Kibana: kbv1.Kibana{
			ObjectMeta: meta,
			Spec: kbv1.KibanaSpec{
				Version: test.Ctx().ElasticStackVersion,
				PodTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						SecurityContext: test.DefaultSecurityContext(),
					},
				},
			},
		},
	}.WithSuffix(randSuffix)
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
	labels := b.Kibana.Spec.PodTemplate.Labels
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[key] = value
	b.Kibana.Spec.PodTemplate.Labels = labels
	return b
}

// -- Helper functions

func (b Builder) RuntimeObjects() []runtime.Object {
	return []runtime.Object{&b.Kibana}
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	commonv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1alpha1"
	kbtype "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/rand"
)

// Builder to create Kibana instances
type Builder struct {
	Kibana kbtype.Kibana
}

var _ test.Builder = Builder{}

func NewBuilder(name string) Builder {
	meta := metav1.ObjectMeta{
		Name:      name,
		Namespace: test.Ctx().ManagedNamespace(0),
	}
	return Builder{
		Kibana: kbtype.Kibana{
			ObjectMeta: meta,
			Spec: kbtype.KibanaSpec{
				Version: test.Ctx().ElasticStackVersion,
				PodTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						SecurityContext: test.DefaultSecurityContext(),
					},
				},
			},
		},
	}.WithSuffix(rand.String(4))
}

func (b Builder) WithSuffix(suffix string) Builder {
	b.Kibana.ObjectMeta.Name = b.Kibana.ObjectMeta.Name + "-" + suffix
	return b
}

func (b Builder) WithElasticsearchRef(ref commonv1alpha1.ObjectSelector) Builder {
	b.Kibana.Spec.ElasticsearchRef = ref
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
	b.Kibana.Spec.NodeCount = int32(count)
	return b
}

func (b Builder) WithKibanaSecureSettings(secretNames ...string) Builder {
	refs := make([]commonv1alpha1.SecretRef, 0, len(secretNames))
	for i := range secretNames {
		refs = append(refs, commonv1alpha1.SecretRef{SecretName: secretNames[i]})
	}
	b.Kibana.Spec.SecureSettings = refs
	return b
}

func (b Builder) WithResources(resources corev1.ResourceRequirements) Builder {
	if len(b.Kibana.Spec.PodTemplate.Spec.Containers) == 0 {
		b.Kibana.Spec.PodTemplate.Spec.Containers = []corev1.Container{
			{Name: kbtype.KibanaContainerName},
		}
	}
	for i, c := range b.Kibana.Spec.PodTemplate.Spec.Containers {
		if c.Name == kbtype.KibanaContainerName {
			c.Resources = resources
			b.Kibana.Spec.PodTemplate.Spec.Containers[i] = c
		}
	}
	return b
}

// -- Helper functions

func (b Builder) RuntimeObjects() []runtime.Object {
	return []runtime.Object{&b.Kibana}
}

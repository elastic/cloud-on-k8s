// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	kbtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Builder to create Kibana instances
type Builder struct {
	Kibana kbtype.Kibana
}

func NewBuilder(name string) Builder {
	meta := metav1.ObjectMeta{
		Name:      name,
		Namespace: test.Namespace,
	}
	return Builder{
		Kibana: kbtype.Kibana{
			ObjectMeta: meta,
			Spec: kbtype.KibanaSpec{
				Version: test.ElasticStackVersion,
				// Create an ElasticsearchRef by default with the same name
				ElasticsearchRef: commonv1alpha1.ObjectSelector{
					Name:      name,
					Namespace: test.Namespace,
				},
				PodTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						SecurityContext: test.DefaultSecurityContext(),
					},
				},
			},
		},
	}
}

// WithRestrictedSecurityContext helps to enforce a restricted security context on the objects.
func (b Builder) WithRestrictedSecurityContext() Builder {
	b.Kibana.Spec.PodTemplate.Spec.SecurityContext = test.DefaultSecurityContext()
	return b
}

func (b Builder) WithNamespace(namespace string) Builder {
	b.Kibana.ObjectMeta.Namespace = namespace
	b.Kibana.Spec.ElasticsearchRef.Namespace = namespace
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

func (b Builder) WithKibanaSecureSettings(secretName string) Builder {
	b.Kibana.Spec.SecureSettings = &commonv1alpha1.SecretRef{
		SecretName: secretName,
	}
	return b
}

// -- Helper functions

func (b Builder) RuntimeObjects() []runtime.Object {
	return []runtime.Object{&b.Kibana}
}

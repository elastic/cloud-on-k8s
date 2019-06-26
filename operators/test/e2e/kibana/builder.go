// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	kbtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/params"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var DefaultResources = corev1.ResourceRequirements{
	Limits: map[corev1.ResourceName]resource.Quantity{
		corev1.ResourceMemory: resource.MustParse("2Gi"),
		corev1.ResourceCPU:    resource.MustParse("2"),
	},
}

func ESPodTemplate(resources corev1.ResourceRequirements) corev1.PodTemplateSpec {
	return corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			SecurityContext: helpers.DefaultSecurityContext(),
			Containers: []corev1.Container{
				{
					Name:      v1alpha1.ElasticsearchContainerName,
					Resources: resources,
				},
			},
		},
	}
}

// -- Stack

type Builder struct {
	Kibana kbtype.Kibana
}

func NewBuilder(name string) Builder {
	meta := metav1.ObjectMeta{
		Name:      name,
		Namespace: params.Namespace,
	}
	return Builder{
		Kibana: kbtype.Kibana{
			ObjectMeta: meta,
			Spec: kbtype.KibanaSpec{
				Version: params.ElasticStackVersion,
				ElasticsearchRef: commonv1alpha1.ObjectSelector{
					Name:      name,
					Namespace: params.Namespace,
				},
				PodTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						SecurityContext: helpers.DefaultSecurityContext(),
					},
				},
			},
		},
	}
}

// WithRestrictedSecurityContext helps to enforce a restricted security context on the objects.
func (b Builder) WithRestrictedSecurityContext() Builder {
	b.Kibana.Spec.PodTemplate.Spec.SecurityContext = helpers.DefaultSecurityContext()
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

func (b Builder) WithKibana(count int) Builder {
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

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserver

import (
	apmtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/apm/v1alpha1"
	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Builder to create APM Servers
type Builder struct {
	ApmServer apmtype.ApmServer
}

func NewBuilder(name string) Builder {
	meta := metav1.ObjectMeta{
		Name:      name,
		Namespace: test.Namespace,
	}
	return Builder{
		ApmServer: apmtype.ApmServer{
			ObjectMeta: meta,
			Spec: apmtype.ApmServerSpec{
				NodeCount: 1,
				Version:   test.ElasticStackVersion,
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

func (b Builder) WithRestrictedSecurityContext() Builder {
	b.ApmServer.Spec.PodTemplate.Spec.SecurityContext = test.DefaultSecurityContext()
	return b
}

func (b Builder) WithNamespace(namespace string) Builder {
	b.ApmServer.ObjectMeta.Namespace = namespace
	b.ApmServer.Spec.ElasticsearchRef.Namespace = namespace
	return b
}

func (b Builder) WithVersion(version string) Builder {
	b.ApmServer.Spec.Version = version
	return b
}

func (b Builder) WithNodeCount(count int) Builder {
	b.ApmServer.Spec.NodeCount = int32(count)
	return b
}

func (b Builder) WithOutput(out apmtype.ElasticsearchOutput) Builder {
	b.ApmServer.Spec.Elasticsearch = out
	return b
}

func (b Builder) WithConfig(cfg map[string]interface{}) Builder {
	b.ApmServer.Spec.Config = &commonv1alpha1.Config{
		Data: cfg,
	}
	return b
}

func (b Builder) WithHTTPCfg(cfg commonv1alpha1.HTTPConfig) Builder {
	b.ApmServer.Spec.HTTP = cfg
	return b
}

// -- Helper functions

func (b Builder) RuntimeObjects() []runtime.Object {
	return []runtime.Object{&b.ApmServer}
}

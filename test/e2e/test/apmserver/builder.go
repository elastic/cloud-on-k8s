// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserver

import (
	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/test/e2e/cmd/run"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/rand"
)

// Builder to create APM Servers
type Builder struct {
	ApmServer      apmv1.ApmServer
	ServiceAccount corev1.ServiceAccount
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

	sa := metav1.ObjectMeta{
		Name:      name,
		Namespace: test.Ctx().ManagedNamespace(0),
	}

	return Builder{
		ServiceAccount: corev1.ServiceAccount{
			ObjectMeta: sa,
		},
		ApmServer: apmv1.ApmServer{
			ObjectMeta: meta,
			Spec: apmv1.ApmServerSpec{
				Count:   1,
				Version: test.Ctx().ElasticStackVersion,
				Config: &commonv1.Config{
					Data: map[string]interface{}{
						"apm-server.ilm.enabled": false,
					},
				},
				PodTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						ServiceAccountName: name,
						SecurityContext:    test.APMDefaultSecurityContext(),
					},
				},
			},
		},
	}.
		WithSuffix(randSuffix).
		WithLabel(run.TestNameLabel, name).
		WithPodLabel(run.TestNameLabel, name)
}

func (b Builder) WithSuffix(suffix string) Builder {
	if suffix != "" {
		b.ApmServer.ObjectMeta.Name = b.ApmServer.ObjectMeta.Name + "-" + suffix
		b.ServiceAccount.ObjectMeta.Name = b.ApmServer.ObjectMeta.GetName()
		b.ApmServer.Spec.PodTemplate.Spec.ServiceAccountName = b.ServiceAccount.GetName()
	}
	return b
}

func (b Builder) WithRestrictedSecurityContext() Builder {
	b.ApmServer.Spec.PodTemplate.Spec.ServiceAccountName = b.ServiceAccount.GetName()
	b.ApmServer.Spec.PodTemplate.Spec.SecurityContext = test.APMDefaultSecurityContext()
	return b
}

func (b Builder) WithNamespace(namespace string) Builder {
	b.ServiceAccount.ObjectMeta.Namespace = namespace
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

// -- Helper functions

func (b Builder) RuntimeObjects() []runtime.Object {
	return []runtime.Object{&b.ServiceAccount, &b.ApmServer}
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

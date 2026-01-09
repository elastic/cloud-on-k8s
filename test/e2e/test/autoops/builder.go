// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	controllerautoops "github.com/elastic/cloud-on-k8s/v3/pkg/controller/autoops"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/cmd/run"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
)

type Builder struct {
	AutoOpsAgentPolicy autoopsv1alpha1.AutoOpsAgentPolicy
	ConfigSecret       corev1.Secret

	Suffix string
}

var _ test.Builder = (*Builder)(nil)

func (b Builder) SkipTest() bool {
	return false
}

func NewBuilder(name string) Builder {
	suffix := rand.String(4)
	meta := metav1.ObjectMeta{
		Name:      name,
		Namespace: test.Ctx().ManagedNamespace(0),
		Labels:    map[string]string{run.TestNameLabel: name},
	}

	configSecretName := "autoops-secret"

	// Configuration secret that will be referenced in policy spec
	configSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configSecretName,
			Namespace: test.Ctx().ManagedNamespace(0),
		},
		StringData: map[string]string{
			"cloud-connected-mode-api-key": "test-api-key",
			// This is being set to localhost to not attempt to send data upstream.
			"autoops-otel-url": "http://localhost:4318",
			"autoops-token":    "test-token",
		},
	}

	return Builder{
		AutoOpsAgentPolicy: autoopsv1alpha1.AutoOpsAgentPolicy{
			ObjectMeta: meta,
			Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
				Version: test.Ctx().ElasticStackVersion,
				ResourceSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"autoops": "enabled",
					},
				},
				AutoOpsRef: autoopsv1alpha1.AutoOpsRef{
					SecretName: configSecretName,
				},
			},
		},
		ConfigSecret: configSecret,
		Suffix:       suffix,
	}.
		WithSuffix(suffix).
		WithLabel(run.TestNameLabel, name)
}

func (b Builder) WithSuffix(suffix string) Builder {
	if suffix != "" {
		b.AutoOpsAgentPolicy.ObjectMeta.Name = b.AutoOpsAgentPolicy.ObjectMeta.Name + "-" + suffix
	}
	return b
}

func (b Builder) WithNamespace(namespace string) Builder {
	b.AutoOpsAgentPolicy.ObjectMeta.Namespace = namespace
	b.ConfigSecret.ObjectMeta.Namespace = namespace
	return b
}

func (b Builder) WithLabel(key, value string) Builder {
	if b.AutoOpsAgentPolicy.ObjectMeta.Labels == nil {
		b.AutoOpsAgentPolicy.ObjectMeta.Labels = make(map[string]string)
	}
	b.AutoOpsAgentPolicy.ObjectMeta.Labels[key] = value
	return b
}

func (b Builder) WithResourceSelector(selector metav1.LabelSelector) Builder {
	b.AutoOpsAgentPolicy.Spec.ResourceSelector = selector
	return b
}

func (b Builder) WithNamespaceSelector(selector metav1.LabelSelector) Builder {
	b.AutoOpsAgentPolicy.Spec.NamespaceSelector = selector
	return b
}

func (b Builder) RuntimeObjects() []k8sclient.Object {
	return []k8sclient.Object{
		&b.ConfigSecret,
		&b.AutoOpsAgentPolicy,
	}
}

func (b Builder) ListOptions() []k8sclient.ListOption {
	return []k8sclient.ListOption{
		k8sclient.InNamespace(b.AutoOpsAgentPolicy.Namespace),
		k8sclient.MatchingLabels(map[string]string{
			controllerautoops.PolicyNameLabelKey: b.AutoOpsAgentPolicy.Name,
		}),
	}
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package esconfig

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	escv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/esconfig/v1alpha1"
	"github.com/elastic/cloud-on-k8s/test/e2e/cmd/run"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

// Builder to create ElasticsearchConfigs
type Builder struct {
	ElasticsearchConfig escv1alpha1.ElasticsearchConfig
	MutatedFrom         *Builder
}

var _ test.Builder = Builder{}

// SkipTest is to satisfy the Builder interface
func (b Builder) SkipTest() bool {
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
		ElasticsearchConfig: escv1alpha1.ElasticsearchConfig{
			ObjectMeta: meta,
			Spec:       escv1alpha1.ElasticsearchConfigSpec{},
		},
	}.
		WithSuffix(randSuffix).
		WithLabel(run.TestNameLabel, name)
}

func (b Builder) WithSuffix(suffix string) Builder {
	if suffix != "" {
		b.ElasticsearchConfig.ObjectMeta.Name = b.ElasticsearchConfig.ObjectMeta.Name + "-" + suffix
	}
	return b
}

func (b Builder) WithElasticsearchRef(ref commonv1.ObjectSelector) Builder {
	b.ElasticsearchConfig.Spec.ElasticsearchRef = ref
	return b
}

func (b Builder) WithNamespace(namespace string) Builder {
	b.ElasticsearchConfig.ObjectMeta.Namespace = namespace
	return b
}

func (b Builder) WithMutatedFrom(mutatedFrom *Builder) Builder {
	b.MutatedFrom = mutatedFrom
	return b
}

func (b Builder) WithLabel(key, value string) Builder {
	if b.ElasticsearchConfig.Labels == nil {
		b.ElasticsearchConfig.Labels = make(map[string]string)
	}
	b.ElasticsearchConfig.Labels[key] = value
	return b
}

// CheckK8sTestSteps exists to satisfy the buider interface. ESConfigs do not create any additional k8s objects to check
func (b Builder) CheckK8sTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{}
}

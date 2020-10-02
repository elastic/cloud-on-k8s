// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package esconfig

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/rand"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	escv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/esconfig/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/test/e2e/cmd/run"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

// Builder to create ElasticsearchConfigs
type Builder struct {
	ElasticsearchConfig escv1alpha1.ElasticsearchConfig
	MutatedFrom         *Builder
	Validations         []ValidationFunc
}

// ValidationFunc is a validation to run against an Elasticsearch cluster
type ValidationFunc func(client.Client) error

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

// WithSLM includes a snapshot repo and SLM config
// https://www.elastic.co/guide/en/elasticsearch/reference/current/getting-started-snapshot-lifecycle-management.html
func (b Builder) WithSLM() Builder {
	b.ElasticsearchConfig.Spec.Operations = []escv1alpha1.ElasticsearchConfigOperation{
		{
			URL: "/_snapshot/my_repository",
			Body: `{
					"type": "fs",
					"settings": {
						"location": "/tmp"
						}
					}`,
		},
		{
			URL: "/_slm/policy/nightly-snapshots",
			Body: `{
					"schedule": "0 30 1 * * ?",
					"name": "<nightly-snap-{now/d}>",
					"repository": "my_repository",
					"config": {
						"indices": ["*"]
					},
					"retention": {
						"expire_after": "30d",
						"min_count": 5,
						"max_count": 50
						}
					}`,
		},
	}
	return b
}

func (b Builder) RuntimeObjects() []runtime.Object {
	return []runtime.Object{&b.ElasticsearchConfig}
}

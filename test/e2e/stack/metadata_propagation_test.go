// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stack

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"reflect"
	"testing"
	"text/template"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	eslabel "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/maps"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/apmserver"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/helper"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/kibana"
)

func TestMetadataPropagation(t *testing.T) {
	builders := mkMetadataPropBuilders(t)

	c := test.NewK8sClientOrFatal()

	want := metadata.Metadata{
		Annotations: map[string]string{"my-annotation": "my-annotation-value"},
		Labels:      map[string]string{"my-label": "my-label-value"},
	}

	steps := func(k *test.K8sClient) test.StepList {
		return []test.Step{
			{
				Name: "check metadata of children",
				Test: func(t *testing.T) {
					t.Helper()
					// nolint:prealloc
					var children []child
					for _, b := range builders {
						expectedChildren, err := expectedChildren(b, c)
						if err != nil {
							t.Fatalf("while fetching expected children for %T: %v", b, err)
						}
						children = append(children, expectedChildren...)
					}
					for _, c := range children {
						c := c
						t.Run(c.identifier(), func(t *testing.T) {
							t.Parallel()
							have := c.metadata(t, k)
							require.True(t, maps.IsSubset(want.Annotations, have.Annotations),
								"Expected annotations not found: \nwant=%++v\nhave=%++v", want.Annotations, have.Annotations)
							require.True(t, maps.IsSubset(want.Labels, have.Labels),
								"Expected labels not found: \nwant=%++v\nhave=%++v", want.Labels, have.Labels)
						})
					}
				},
			},
		}
	}

	test.Sequence(nil, steps, builders...).RunSequential(t)
}

func mkMetadataPropBuilders(t *testing.T) []test.Builder {
	t.Helper()

	tmpl, err := template.ParseFiles("testdata/metadata_propagation.yaml")
	require.NoError(t, err, "Failed to parse template")

	buf := new(bytes.Buffer)
	rndSuffix := rand.String(4)

	require.NoError(t, tmpl.Execute(buf, map[string]string{
		"Suffix": rndSuffix,
	}))

	namespace := test.Ctx().ManagedNamespace(0)
	stackVersion := test.Ctx().ElasticStackVersion

	transform := func(builder test.Builder) test.Builder {
		switch b := builder.(type) {
		case elasticsearch.Builder:
			return b.WithNamespace(namespace).
				WithVersion(stackVersion).
				WithRestrictedSecurityContext()
		case kibana.Builder:
			return b.WithNamespace(namespace).
				WithVersion(stackVersion).
				WithRestrictedSecurityContext()
		case apmserver.Builder:
			return b.WithNamespace(namespace).
				WithVersion(stackVersion).
				WithRestrictedSecurityContext()
		default:
			return b
		}
	}

	decoder := helper.NewYAMLDecoder()
	builders, err := decoder.ToBuilders(bufio.NewReader(buf), transform)
	require.NoError(t, err, "Failed to create builders")

	return builders
}

func expectedChildren(builder test.Builder, c *test.K8sClient) ([]child, error) {
	switch b := builder.(type) {
	case elasticsearch.Builder:
		return expectedChildrenForElasticsearch(b, c)
	default:
		return nil, nil
	}
}

func expectedChildrenForElasticsearch(b elasticsearch.Builder, c *test.K8sClient) ([]child, error) {
	ns := b.Elasticsearch.Namespace
	name := b.Elasticsearch.Name

	children := make([]child, 0, 20) // preallocate some space for children
	matchLabels := client.MatchingLabels(map[string]string{
		eslabel.ClusterNameLabelName: name,
		v1.TypeLabelName:             "elasticsearch",
	})
	for _, list := range []client.ObjectList{
		&corev1.ServiceList{},
		&corev1.SecretList{},
		&corev1.ConfigMapList{},
		&appsv1.StatefulSetList{},
		&corev1.PodList{},
		&policyv1.PodDisruptionBudgetList{},
	} {
		if err := c.Client.List(context.Background(), list, client.InNamespace(ns), matchLabels); err != nil {
			return nil, err
		}
		// Use reflection to get the Items field generically.
		v := reflect.ValueOf(list).Elem().FieldByName("Items")
		if !v.IsValid() {
			return nil, fmt.Errorf("no Items field found in %T", list)
		}
		if v.Len() == 0 {
			return nil, fmt.Errorf("empty list returned by client, no items found for type %T", list)
		}
		for i := 0; i < v.Len(); i++ {
			item, ok := v.Index(i).Addr().Interface().(client.Object)
			if !ok {
				return nil, fmt.Errorf("item %d in list %T is not a client.Object", i, list)
			}
			children = append(children, child{
				parent: "Elasticsearch",
				key:    client.ObjectKey{Namespace: item.GetNamespace(), Name: item.GetName()},
				obj: func(obj client.Object) func() client.Object {
					return func() client.Object {
						return obj.DeepCopyObject().(client.Object) //nolint:forcetypeassert
					}
				}(item),
			})
		}
	}
	return children, nil
}

type child struct {
	parent string
	key    client.ObjectKey
	obj    func() client.Object
}

func (c child) identifier() string {
	return fmt.Sprintf("%s/%T/%s", c.parent, c.obj(), c.key.String())
}

func (c child) metadata(t *testing.T, k *test.K8sClient) metadata.Metadata {
	t.Helper()
	t.Logf("Getting %s", c.identifier())

	obj := c.obj()

	err := k.Client.Get(context.Background(), c.key, obj)
	require.NoError(t, err, "Failed to get object")

	accessor := meta.NewAccessor()

	haveAnnotations, err := accessor.Annotations(obj)
	require.NoError(t, err, "Failed to get annotations")

	haveLabels, err := accessor.Labels(obj)
	require.NoError(t, err, "Failed to get labels")

	return metadata.Metadata{Annotations: haveAnnotations, Labels: haveLabels}
}

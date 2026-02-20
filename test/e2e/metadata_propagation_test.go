// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package e2e

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/hashicorp/go-multierror"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/agent"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	eslabel "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	kblabel "github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana/label"
	lslabels "github.com/elastic/cloud-on-k8s/v3/pkg/controller/logstash/labels"
	emslabels "github.com/elastic/cloud-on-k8s/v3/pkg/controller/maps"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/maps"
	e2e_agent "github.com/elastic/cloud-on-k8s/v3/test/e2e/agent"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	elasticagent "github.com/elastic/cloud-on-k8s/v3/test/e2e/test/agent"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/beat"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/kibana"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/logstash"
	ems "github.com/elastic/cloud-on-k8s/v3/test/e2e/test/maps"
)

func TestMetadataPropagation(t *testing.T) {
	c := test.NewK8sClientOrFatal()

	want := metadata.Metadata{
		Annotations: map[string]string{"my-annotation": "my-annotation-value"},
		Labels:      map[string]string{"my-label": "my-label-value"},
	}

	name := "test-meta-prop"
	es := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithLabel("my-label", "my-label-value").
		WithAnnotation("eck.k8s.alpha.elastic.co/propagate-annotations", "*").
		WithAnnotation("eck.k8s.alpha.elastic.co/propagate-labels", "*").
		WithAnnotation("my-annotation", "my-annotation-value")
	kb := kibana.NewBuilder(name).
		WithNodeCount(1).
		WithElasticsearchRef(es.Ref()).
		WithLabel("my-label", "my-label-value").
		WithAnnotation("eck.k8s.alpha.elastic.co/propagate-annotations", "*").
		WithAnnotation("eck.k8s.alpha.elastic.co/propagate-labels", "*").
		WithAnnotation("my-annotation", "my-annotation-value")
	testPod := beat.NewPodBuilder(name)
	agent := elasticagent.NewBuilder(name).
		WithElasticsearchRefs(elasticagent.ToOutput(es.Ref(), "default")).
		WithDefaultESValidation(elasticagent.HasWorkingDataStream(elasticagent.LogsType, "elastic_agent", "default")).
		WithOpenShiftRoles(test.UseSCCRole).
		WithLabel("my-label", "my-label-value").
		WithAnnotation("eck.k8s.alpha.elastic.co/propagate-annotations", "*").
		WithAnnotation("eck.k8s.alpha.elastic.co/propagate-labels", "*").
		WithAnnotation("my-annotation", "my-annotation-value").
		MoreResourcesForIssue4730()
	agent = elasticagent.ApplyYamls(t, agent, e2e_agent.E2EAgentSystemIntegrationConfig, e2e_agent.E2EAgentSystemIntegrationPodTemplate)
	ls := logstash.NewBuilder(name).
		WithRestrictedSecurityContext().
		WithNodeCount(1).
		WithElasticsearchRefs(
			logstashv1alpha1.ElasticsearchCluster{
				ObjectSelector: es.Ref(),
				ClusterName:    "es",
			}).
		WithLabel("my-label", "my-label-value").
		WithAnnotation("eck.k8s.alpha.elastic.co/propagate-annotations", "*").
		WithAnnotation("eck.k8s.alpha.elastic.co/propagate-labels", "*").
		WithAnnotation("my-annotation", "my-annotation-value")
	emsBuilder := ems.NewBuilder(name).
		WithNodeCount(1).
		WithElasticsearchRef(es.Ref()).
		WithRestrictedSecurityContext().
		WithLabel("my-label", "my-label-value").
		WithAnnotation("eck.k8s.alpha.elastic.co/propagate-annotations", "*").
		WithAnnotation("eck.k8s.alpha.elastic.co/propagate-labels", "*").
		WithAnnotation("my-annotation", "my-annotation-value")

	esWithLicense := test.LicenseTestBuilder(es)

	builders := []test.Builder{esWithLicense, emsBuilder, kb, agent, ls, testPod}

	steps := func(k *test.K8sClient) test.StepList {
		return []test.Step{
			{
				Name: "check metadata of children",
				Test: func(t *testing.T) {
					t.Helper()

					children := make([]child, 0, len(builders))
					for _, b := range builders {
						expectedChildren, err := expectedChildren(b, c)
						if err != nil {
							t.Fatalf("while fetching expected children for %T: %v", b, err)
						}
						children = append(children, expectedChildren...)
					}
					for _, c := range children {
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

func expectedChildren(builder test.Builder, c *test.K8sClient) ([]child, error) {
	switch b := builder.(type) {
	case test.WrappedBuilder:
		// If the builder is wrapped, we need to unwrap it to get the actual builder.
		return expectedChildren(b.BuildingThis, c)
	case elasticsearch.Builder:
		return expectedChildrenFor(c, "elasticsearch", b.Elasticsearch.Namespace).
			GetObjects(map[string]string{eslabel.ClusterNameLabelName: b.Elasticsearch.Name, v1.TypeLabelName: "elasticsearch"}, &corev1.ServiceList{}, &corev1.SecretList{}, &corev1.ConfigMapList{}, &appsv1.StatefulSetList{}, &corev1.PodList{}, &policyv1.PodDisruptionBudgetList{}).
			Result()
	case kibana.Builder:
		return expectedChildrenFor(c, "kibana", b.Kibana.Namespace).
			GetObjects(map[string]string{kblabel.KibanaNameLabelName: b.Kibana.Name, v1.TypeLabelName: "kibana"}, &corev1.ServiceList{}, &corev1.SecretList{}, &corev1.ConfigMapList{}, &appsv1.DeploymentList{}, &corev1.PodList{}).
			// Also check that the Secrets metadata from the association with Elasticsearch
			GetObjects(map[string]string{"kibanaassociation.k8s.elastic.co/type": "elasticsearch"}, &corev1.SecretList{}).
			Result()
	case elasticagent.Builder:
		return expectedChildrenFor(c, "agent", b.Agent.Namespace).
			GetObjects(map[string]string{agent.NameLabelName: b.Agent.Name, v1.TypeLabelName: "agent"}, &corev1.SecretList{}, &appsv1.DaemonSetList{}, &corev1.PodList{}).
			// Also check that the Secrets metadata from the association with Elasticsearch
			GetObjects(map[string]string{"agentassociation.k8s.elastic.co/type": "elasticsearch"}, &corev1.SecretList{}).
			Result()
	case logstash.Builder:
		return expectedChildrenFor(c, "logstash", b.Logstash.Namespace).
			GetObjects(map[string]string{lslabels.NameLabelName: b.Logstash.Name, v1.TypeLabelName: "logstash"}, &corev1.ServiceList{}, &corev1.SecretList{}, &appsv1.StatefulSetList{}, &corev1.PodList{}).
			// Also check that the Secrets metadata from the association with Elasticsearch
			GetObjects(map[string]string{"logstashassociation.k8s.elastic.co/type": "elasticsearch"}, &corev1.SecretList{}).
			Result()
	case ems.Builder:
		return expectedChildrenFor(c, "maps", b.EMS.Namespace).
			GetObjects(map[string]string{emslabels.NameLabelName: b.EMS.Name, v1.TypeLabelName: "maps"}, &corev1.ServiceList{}, &corev1.SecretList{}, &appsv1.DeploymentList{}, &corev1.PodList{}).
			// Also check that the Secrets metadata from the association with Elasticsearch
			GetObjects(map[string]string{"mapsassociation.k8s.elastic.co/type": "elasticsearch"}, &corev1.SecretList{}).
			Result()
	default:
		return nil, nil
	}
}

// -- Helper functions

func expectedChildrenFor(c *test.K8sClient, parentType, namespace string) *expectedChildrenForHelper {
	return &expectedChildrenForHelper{
		c:          c,
		parentType: parentType,
		namespace:  namespace,
		children:   make([]child, 0, 20), // preallocate some space for children
	}
}

type expectedChildrenForHelper struct {
	c                     *test.K8sClient
	parentType, namespace string
	children              []child
	err                   error
}

func (ec *expectedChildrenForHelper) Result() ([]child, error) {
	if ec.err != nil {
		return nil, ec.err
	}
	return ec.children, nil
}

func (ec *expectedChildrenForHelper) GetObjects(matchingLabels map[string]string, objects ...client.ObjectList) *expectedChildrenForHelper {
	for _, list := range objects {
		if err := ec.c.Client.List(context.Background(), list, client.InNamespace(ec.namespace), client.MatchingLabels(matchingLabels)); err != nil {
			ec.err = multierror.Append(ec.err, err)
			return ec
		}
		// Use reflection to get the Items field generically.
		v := reflect.ValueOf(list).Elem().FieldByName("Items")
		if !v.IsValid() {
			ec.err = multierror.Append(ec.err, fmt.Errorf("no Items field found in %T", list))
			return ec
		}
		if v.Len() == 0 {
			ec.err = multierror.Append(ec.err, fmt.Errorf("empty list returned by client, no items found for type %T", list))
			return ec
		}
		for i := 0; i < v.Len(); i++ {
			item, ok := v.Index(i).Addr().Interface().(client.Object)
			if !ok {
				ec.err = multierror.Append(ec.err, fmt.Errorf("item %d in list %T is not a client.Object", i, list))
				return ec
			}
			ec.children = append(ec.children, child{
				parent: ec.parentType,
				key:    client.ObjectKey{Namespace: item.GetNamespace(), Name: item.GetName()},
				obj: func(obj client.Object) func() client.Object {
					return func() client.Object {
						return obj.DeepCopyObject().(client.Object) //nolint:forcetypeassert
					}
				}(item),
			})
		}
	}
	return ec
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

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stack

import (
	"bufio"
	"bytes"
	"fmt"
	"testing"
	"text/template"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/utils/maps"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/apmserver"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/helper"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/kibana"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestMetadataPropagation(t *testing.T) {
	builders := mkMetadataPropBuilders(t)

	var children []child
	for _, b := range builders {
		children = append(children, expectedChildren(b)...)
	}

	want := metadata.Metadata{
		Annotations: map[string]string{"my-annotation": "my-annotation-value"},
		Labels:      map[string]string{"my-label": "my-label-value"},
	}

	steps := func(k *test.K8sClient) test.StepList {
		return []test.Step{
			{
				Name: "check metadata of children",
				Test: func(t *testing.T) {
					k, err := test.NewK8sClient()
					require.NoError(t, err, "Failed to create new Kube client")

					for _, c := range children {
						c := c
						t.Run(c.key.String(), func(t *testing.T) {
							t.Parallel()

							have := c.getMetadata(t, k)
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

func expectedChildren(builder test.Builder) []child {
	switch b := builder.(type) {
	case elasticsearch.Builder:
		return expectedChidrenForElasticsearch(b)
	case kibana.Builder:
		return expectedChidrenForKibana(b)
	case apmserver.Builder:
		return expectedChidrenForAPMServer(b)
	default:
		return nil
	}
}

func expectedChidrenForElasticsearch(b elasticsearch.Builder) []child {
	ns := b.Elasticsearch.Namespace
	name := b.Elasticsearch.Name
	children := []child{
		{
			key: client.ObjectKey{Namespace: ns, Name: esv1.ElasticUserSecret(name)},
			obj: func() runtime.Object { return &corev1.Secret{} },
		},
		{
			key: client.ObjectKey{Namespace: ns, Name: esv1.HTTPService(name)},
			obj: func() runtime.Object { return &corev1.Service{} },
		},
		{
			key: client.ObjectKey{Namespace: ns, Name: certificates.CAInternalSecretName(esv1.ESNamer, name, certificates.HTTPCAType)},
			obj: func() runtime.Object { return &corev1.Secret{} },
		},
		{
			key: client.ObjectKey{Namespace: ns, Name: certificates.InternalCertsSecretName(esv1.ESNamer, name)},
			obj: func() runtime.Object { return &corev1.Secret{} },
		},
		{
			key: client.ObjectKey{Namespace: ns, Name: certificates.PublicCertsSecretName(esv1.ESNamer, name)},
			obj: func() runtime.Object { return &corev1.Secret{} },
		},
		{
			key: client.ObjectKey{Namespace: ns, Name: esv1.InternalUsersSecret(name)},
			obj: func() runtime.Object { return &corev1.Secret{} },
		},
		{
			key: client.ObjectKey{Namespace: ns, Name: esv1.RemoteCaSecretName(name)},
			obj: func() runtime.Object { return &corev1.Secret{} },
		},
		{
			key: client.ObjectKey{Namespace: ns, Name: esv1.ScriptsConfigMap(name)},
			obj: func() runtime.Object { return &corev1.ConfigMap{} },
		},
		{
			key: client.ObjectKey{Namespace: ns, Name: esv1.TransportService(name)},
			obj: func() runtime.Object { return &corev1.Service{} },
		},
		{
			key: client.ObjectKey{Namespace: ns, Name: certificates.CAInternalSecretName(esv1.ESNamer, name, certificates.TransportCAType)},
			obj: func() runtime.Object { return &corev1.Secret{} },
		},
		{
			key: client.ObjectKey{Namespace: ns, Name: esv1.TransportCertificatesSecret(name)},
			obj: func() runtime.Object { return &corev1.Secret{} },
		},
		{
			key: client.ObjectKey{Namespace: ns, Name: certificates.PublicTransportCertsSecretName(esv1.ESNamer, name)},
			obj: func() runtime.Object { return &corev1.Secret{} },
		},
		{
			key: client.ObjectKey{Namespace: ns, Name: esv1.UnicastHostsConfigMap(name)},
			obj: func() runtime.Object { return &corev1.ConfigMap{} },
		},
		{
			key: client.ObjectKey{Namespace: ns, Name: esv1.RolesAndFileRealmSecret(name)},
			obj: func() runtime.Object { return &corev1.Secret{} },
		},
	}

	for _, nodeSet := range b.Elasticsearch.Spec.NodeSets {
		ssetName := esv1.StatefulSet(name, nodeSet.Name)
		stsChildren := []child{
			{
				key: client.ObjectKey{Namespace: ns, Name: ssetName},
				obj: func() runtime.Object { return &appsv1.StatefulSet{} },
			},
			{
				key: client.ObjectKey{Namespace: ns, Name: nodespec.HeadlessServiceName(ssetName)},
				obj: func() runtime.Object { return &corev1.Service{} },
			},
			{
				key: client.ObjectKey{Namespace: ns, Name: esv1.ConfigSecret(ssetName)},
				obj: func() runtime.Object { return &corev1.Secret{} },
			},
		}

		for i := 0; i < int(nodeSet.Count); i++ {
			stsChildren = append(stsChildren, child{
				key: client.ObjectKey{Namespace: ns, Name: fmt.Sprintf("%s-%d", ssetName, i)},
				obj: func() runtime.Object { return &corev1.Pod{} },
			})
		}

		children = append(children, stsChildren...)
	}

	return children
}

func expectedChidrenForKibana(b kibana.Builder) []child {
	return nil
}

func expectedChidrenForAPMServer(b apmserver.Builder) []child {
	return nil
}

type child struct {
	key client.ObjectKey
	obj func() runtime.Object
}

func (c child) getMetadata(t *testing.T, k *test.K8sClient) metadata.Metadata {
	t.Helper()
	t.Logf("Getting %s", c.key.String())

	obj := c.obj()
	err := k.Client.Get(c.key, obj)
	require.NoError(t, err, "Failed to get object")

	accessor := meta.NewAccessor()

	haveAnnotations, err := accessor.Annotations(obj)
	require.NoError(t, err, "Failed to get annotations")

	haveLabels, err := accessor.Labels(obj)
	require.NoError(t, err, "Failed to get labels")

	return metadata.Metadata{Annotations: haveAnnotations, Labels: haveLabels}
}

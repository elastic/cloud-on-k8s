// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/go-test/deep"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/observer"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

func bootstrappedES() *v1alpha1.Elasticsearch {
	return &v1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "cluster",
			Annotations: map[string]string{ClusterUUIDAnnotationName: "uuid"},
		},
		Spec: v1alpha1.ElasticsearchSpec{Version: "7.3.0"},
	}
}

func notBootstrappedES() *v1alpha1.Elasticsearch {
	return &v1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec:       v1alpha1.ElasticsearchSpec{Version: "7.3.0"},
	}
}

func TestAnnotatedForBootstrap(t *testing.T) {
	require.True(t, AnnotatedForBootstrap(*bootstrappedES()))
	require.False(t, AnnotatedForBootstrap(*notBootstrappedES()))
}

func Test_annotateWithUUID(t *testing.T) {
	require.NoError(t, v1alpha1.AddToScheme(scheme.Scheme))

	cluster := notBootstrappedES()
	observedState := observer.State{ClusterState: &client.ClusterState{ClusterUUID: "cluster-uuid"}}
	k8sClient := k8s.WrapClient(fake.NewFakeClient(cluster))

	err := annotateWithUUID(cluster, observedState, k8sClient)
	require.NoError(t, err)
	require.True(t, AnnotatedForBootstrap(*cluster))

	var retrieved v1alpha1.Elasticsearch
	err = k8sClient.Get(k8s.ExtractNamespacedName(cluster), &retrieved)
	require.NoError(t, err)
	require.True(t, AnnotatedForBootstrap(retrieved))
}

func TestReconcileClusterUUID(t *testing.T) {
	require.NoError(t, v1alpha1.AddToScheme(scheme.Scheme))
	tests := []struct {
		name          string
		c             k8s.Client
		cluster       *v1alpha1.Elasticsearch
		observedState observer.State
		wantCluster   *v1alpha1.Elasticsearch
	}{
		{
			name:        "already annotated",
			c:           k8s.WrapClient(fake.NewFakeClient()),
			cluster:     bootstrappedES(),
			wantCluster: bootstrappedES(),
		},
		{
			name:          "not annotated, but not bootstrapped yet (cluster state empty)",
			cluster:       notBootstrappedES(),
			c:             k8s.WrapClient(fake.NewFakeClient()),
			observedState: observer.State{ClusterState: nil},
			wantCluster:   notBootstrappedES(),
		},
		{
			name:          "not annotated, but not bootstrapped yet (cluster UUID empty)",
			cluster:       notBootstrappedES(),
			c:             k8s.WrapClient(fake.NewFakeClient()),
			observedState: observer.State{ClusterState: &client.ClusterState{ClusterUUID: ""}},
			wantCluster:   notBootstrappedES(),
		},
		{
			name:          "not annotated, but bootstrapped",
			c:             k8s.WrapClient(fake.NewFakeClient(notBootstrappedES())),
			cluster:       notBootstrappedES(),
			observedState: observer.State{ClusterState: &client.ClusterState{ClusterUUID: "uuid"}},
			wantCluster:   bootstrappedES(),
		},
		{
			name: "annotated, bootstrapped, but needs re-bootstrapping due to single node upgrade ",
			c: k8s.WrapClient(fake.NewFakeClient(
				bootstrappedES(),
				sset.TestPod{
					ClusterName: "cluster",
					Version:     "6.8.0",
					Master:      true,
				}.BuildPtr())),
			cluster:       bootstrappedES(),
			observedState: observer.State{ClusterState: &client.ClusterState{ClusterUUID: "uuid"}},
			wantCluster: func() *v1alpha1.Elasticsearch {
				es := bootstrappedES().DeepCopy()
				es.Annotations = make(map[string]string) // simulate removed annotation
				return es
			}(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ReconcileClusterUUID(tt.c, tt.cluster, tt.observedState)
			require.NoError(t, err)
			require.Nil(t, deep.Equal(tt.wantCluster, tt.cluster))
		})
	}
}

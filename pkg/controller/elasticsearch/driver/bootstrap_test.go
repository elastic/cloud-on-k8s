// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/observer"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/go-test/deep"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func bootstrappedES() *esv1.Elasticsearch {
	return &esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "cluster",
			Annotations: map[string]string{ClusterUUIDAnnotationName: "uuid"},
		},
		Spec: esv1.ElasticsearchSpec{Version: "7.3.0"},
	}
}

func bootstrappedESWithChangeBudget(maxSurge, maxUnavailable *int32) *esv1.Elasticsearch {
	es := bootstrappedES()
	es.Spec.UpdateStrategy = esv1.UpdateStrategy{
		ChangeBudget: esv1.ChangeBudget{
			MaxSurge:       maxSurge,
			MaxUnavailable: maxUnavailable,
		},
	}

	return es
}

func notBootstrappedES() *esv1.Elasticsearch {
	return &esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec:       esv1.ElasticsearchSpec{Version: "7.3.0"},
	}
}

func reBootstrappingES() *esv1.Elasticsearch {
	return &esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "cluster",
			Annotations: map[string]string{},
		},
		Spec: esv1.ElasticsearchSpec{Version: "7.3.0"},
	}
}

func TestAnnotatedForBootstrap(t *testing.T) {
	require.True(t, AnnotatedForBootstrap(*bootstrappedES()))
	require.False(t, AnnotatedForBootstrap(*notBootstrappedES()))
}

func Test_annotateWithUUID(t *testing.T) {
	cluster := notBootstrappedES()
	observedState := observer.State{ClusterInfo: &client.Info{ClusterUUID: "cluster-uuid"}}
	k8sClient := k8s.WrappedFakeClient(cluster)

	err := annotateWithUUID(cluster, observedState, k8sClient)
	require.NoError(t, err)
	require.True(t, AnnotatedForBootstrap(*cluster))

	var retrieved esv1.Elasticsearch
	err = k8sClient.Get(k8s.ExtractNamespacedName(cluster), &retrieved)
	require.NoError(t, err)
	require.True(t, AnnotatedForBootstrap(retrieved))
}

func Test_clusterIsBootstrapped(t *testing.T) {
	tests := []struct {
		name  string
		state observer.State
		want  bool
	}{
		{
			name:  "empty state",
			state: observer.State{},
			want:  false,
		},
		{
			name:  "cluster uuid empty",
			state: observer.State{ClusterInfo: &esclient.Info{}},
			want:  false,
		},
		{
			name:  "cluster uuid _na_ (not available) yet, cluster is still forming",
			state: observer.State{ClusterInfo: &esclient.Info{ClusterUUID: "_na_"}},
			want:  false,
		},
		{
			name:  "cluster uuid set, cluster bootstrapped",
			state: observer.State{ClusterInfo: &esclient.Info{ClusterUUID: "6902c192-ec1d-11e9-81b4-2a2ae2dbcce4"}},
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, clusterIsBootstrapped(tt.state))
		})
	}
}

func TestReconcileClusterUUID(t *testing.T) {
	tests := []struct {
		name          string
		c             k8s.Client
		cluster       *esv1.Elasticsearch
		observedState observer.State
		wantCluster   *esv1.Elasticsearch
	}{
		{
			name:        "already annotated",
			c:           k8s.WrappedFakeClient(),
			cluster:     bootstrappedES(),
			wantCluster: bootstrappedES(),
		},
		{
			name:          "not annotated, but not bootstrapped yet (cluster state empty)",
			cluster:       notBootstrappedES(),
			c:             k8s.WrappedFakeClient(),
			observedState: observer.State{ClusterInfo: nil},
			wantCluster:   notBootstrappedES(),
		},
		{
			name:          "not annotated, but not bootstrapped yet (cluster UUID empty)",
			cluster:       notBootstrappedES(),
			c:             k8s.WrappedFakeClient(),
			observedState: observer.State{ClusterInfo: &client.Info{ClusterUUID: ""}},
			wantCluster:   notBootstrappedES(),
		},
		{
			name:          "not annotated, but bootstrapped",
			c:             k8s.WrappedFakeClient(notBootstrappedES()),
			cluster:       notBootstrappedES(),
			observedState: observer.State{ClusterInfo: &client.Info{ClusterUUID: "uuid"}},
			wantCluster:   bootstrappedES(),
		},
		{
			name: "annotated, bootstrapped, but needs re-bootstrapping due to single node upgrade ",
			c: k8s.WrappedFakeClient(
				bootstrappedES(),
				sset.TestPod{
					ClusterName: "cluster",
					Version:     "6.8.0",
					Master:      true,
				}.BuildPtr()),
			cluster:       bootstrappedES(),
			observedState: observer.State{ClusterInfo: &client.Info{ClusterUUID: "uuid"}},
			wantCluster:   reBootstrappingES(),
		},
		{
			name: "not annotated, bootstrapped, but still on pre-upgrade version",
			c: k8s.WrappedFakeClient(
				reBootstrappingES(),
				sset.TestPod{
					ClusterName: "cluster",
					Version:     "6.8.0",
					Master:      true,
				}.BuildPtr(),
			),
			cluster:       reBootstrappingES(),
			observedState: observer.State{ClusterInfo: &client.Info{ClusterUUID: "uuid"}},
			wantCluster:   reBootstrappingES(),
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

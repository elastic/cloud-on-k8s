// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package zen2

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	commonsettings "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
	sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/bootstrap"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func esv7() esv1.Elasticsearch {
	return esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"},
		Spec:       esv1.ElasticsearchSpec{Version: "7.5.0"},
	}
}
func withAnnotations(es esv1.Elasticsearch, annotations map[string]string) esv1.Elasticsearch {
	esCopy := es.DeepCopy()
	esCopy.Annotations = annotations
	return *esCopy
}

func TestSetupInitialMasterNodes(t *testing.T) {
	expectedv7resources := func() nodespec.ResourcesList {
		return nodespec.ResourcesList{
			{StatefulSet: sset.TestSset{Name: "es-master", Version: "7.5.0", Replicas: 3, Master: true, Data: false, ClusterName: "es"}.Build(), Config: settings.NewCanonicalConfig()},
			{StatefulSet: sset.TestSset{Name: "es-masterdata", Version: "7.5.0", Replicas: 3, Master: true, Data: true, ClusterName: "es"}.Build(), Config: settings.NewCanonicalConfig()},
			{StatefulSet: sset.TestSset{Name: "es-data", Version: "7.5.0", Replicas: 3, Master: false, Data: true, ClusterName: "es"}.Build(), Config: settings.NewCanonicalConfig()},
		}
	}
	tests := []struct {
		name               string
		nodeSpecResources  nodespec.ResourcesList
		k8sClient          k8s.Client
		es                 esv1.Elasticsearch
		expectedConfigs    []settings.CanonicalConfig
		expectedAnnotation string
	}{
		{
			name:              "v7 cluster initial creation: compute and set cluster.initial_master_nodes",
			es:                esv7(),
			nodeSpecResources: expectedv7resources(),
			k8sClient:         k8s.NewFakeClient(),
			expectedConfigs: []settings.CanonicalConfig{
				// master nodes config
				{CanonicalConfig: commonsettings.MustCanonicalConfig(map[string][]string{
					esv1.ClusterInitialMasterNodes: {"es-master-0", "es-master-1", "es-master-2", "es-masterdata-0", "es-masterdata-1", "es-masterdata-2"},
				})},
				// master + data nodes config
				{CanonicalConfig: commonsettings.MustCanonicalConfig(map[string][]string{
					esv1.ClusterInitialMasterNodes: {"es-master-0", "es-master-1", "es-master-2", "es-masterdata-0", "es-masterdata-1", "es-masterdata-2"},
				})},
				// no config set on non-data nodes
				{CanonicalConfig: commonsettings.NewCanonicalConfig()},
			},
			expectedAnnotation: "es-master-0,es-master-1,es-master-2,es-masterdata-0,es-masterdata-1,es-masterdata-2",
		},
		{
			name: "v7 cluster currently bootstrapping: reuse the annotated cluster.initial_master_nodes value for master nodes",
			// initial master node names do not match the "real" node names: that's on purpose so we make sure
			// those "fake" node values are the ones being reused
			es:                withAnnotations(esv7(), map[string]string{InitialMasterNodesAnnotation: "node-0,node-1,node-2"}),
			nodeSpecResources: expectedv7resources(),
			k8sClient:         k8s.NewFakeClient(),
			expectedConfigs: []settings.CanonicalConfig{
				// master nodes config
				{CanonicalConfig: commonsettings.MustCanonicalConfig(map[string][]string{
					esv1.ClusterInitialMasterNodes: {"node-0", "node-1", "node-2"},
				})},
				// master + data nodes config
				{CanonicalConfig: commonsettings.MustCanonicalConfig(map[string][]string{
					esv1.ClusterInitialMasterNodes: {"node-0", "node-1", "node-2"},
				})},
				// no config set on non-data nodes
				{CanonicalConfig: commonsettings.NewCanonicalConfig()},
			},
			// annotation should be kept the same
			expectedAnnotation: "node-0,node-1,node-2",
		},
		{
			name: "v7 cluster existed before: nothing to do",
			// set the ClusterUUID annotation to indicate the cluster did form in the past, so
			// cluster.initial_master_nodes should not be set
			es:                 withAnnotations(esv7(), map[string]string{bootstrap.ClusterUUIDAnnotationName: "uuid"}),
			nodeSpecResources:  expectedv7resources(),
			k8sClient:          k8s.NewFakeClient(),
			expectedConfigs:    []settings.CanonicalConfig{settings.NewCanonicalConfig(), settings.NewCanonicalConfig(), settings.NewCanonicalConfig()},
			expectedAnnotation: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, tt.k8sClient.Create(context.Background(), &tt.es))
			err := SetupInitialMasterNodes(context.Background(), tt.es, tt.k8sClient, tt.nodeSpecResources)
			require.NoError(t, err)
			// nodeSpecResources configurations should be updated accordingly
			for i := 0; i < len(tt.nodeSpecResources); i++ {
				expected, err := tt.expectedConfigs[i].Render()
				require.NoError(t, err)
				actual, err := tt.nodeSpecResources[i].Config.Render()
				require.NoError(t, err)
				require.Equal(t, expected, actual)
			}
			// es annotation should be set accordingly
			var updatedEs esv1.Elasticsearch
			err = tt.k8sClient.Get(context.Background(), k8s.ExtractNamespacedName(&tt.es), &updatedEs)
			require.NoError(t, err)
			if tt.expectedAnnotation != "" {
				require.Equal(t, tt.expectedAnnotation, updatedEs.Annotations[InitialMasterNodesAnnotation])
			}
		})
	}
}

func Test_getInitialMasterNodesAnnotation(t *testing.T) {
	tests := []struct {
		name string
		es   esv1.Elasticsearch
		want []string
	}{
		{
			name: "annotations nil",
			es:   esv1.Elasticsearch{},
			want: nil,
		},
		{
			name: "annotation not set",
			es: esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
				"foo": "bar",
			}}},
			want: nil,
		},
		{
			name: "annotation set with 1 master node",
			es: esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
				InitialMasterNodesAnnotation: "node-0",
			}}},
			want: []string{"node-0"},
		},
		{
			name: "annotation set with several master nodes",
			es: esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
				InitialMasterNodesAnnotation: "node-0,node-1,node-2",
			}}},
			want: []string{"node-0", "node-1", "node-2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getInitialMasterNodesAnnotation(tt.es); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getInitialMasterNodesAnnotation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_setInitialMasterNodesAnnotation(t *testing.T) {
	es := esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"}}
	k8sClient := k8s.NewFakeClient(&es)
	initialMasterNodes := []string{"node-0", "node-1", "node-2"}
	err := setInitialMasterNodesAnnotation(context.Background(), k8sClient, es, initialMasterNodes)
	require.NoError(t, err)
	var updatedEs esv1.Elasticsearch
	err = k8sClient.Get(context.Background(), k8s.ExtractNamespacedName(&es), &updatedEs)
	require.NoError(t, err)
	require.Equal(t, "node-0,node-1,node-2", updatedEs.Annotations[InitialMasterNodesAnnotation])
}

type mockZen2BootstrapESClient struct {
	zen2Bootstrapped bool
	err              error
	client.Client
}

func (f *mockZen2BootstrapESClient) ClusterBootstrappedForZen2(_ context.Context) (bool, error) {
	return f.zen2Bootstrapped, f.err
}

func TestRemoveZen2BootstrapAnnotation(t *testing.T) {
	type args struct {
		es       esv1.Elasticsearch
		esClient client.Client
	}
	tests := []struct {
		name           string
		args           args
		wantRequeue    bool
		wantAnnotation bool
	}{
		{
			name: "v7 cluster with no annotation: nothing to do",
			args: args{
				es:       esv7(),
				esClient: &mockZen2BootstrapESClient{},
			},
			wantRequeue:    false,
			wantAnnotation: false,
		},
		{
			name: "v7 cluster with annotation but bootstrap not over yet: requeue & keep annotation",
			args: args{
				es:       withAnnotations(esv7(), map[string]string{InitialMasterNodesAnnotation: "foo,bar"}),
				esClient: &mockZen2BootstrapESClient{zen2Bootstrapped: false, err: nil},
			},
			wantRequeue:    true,
			wantAnnotation: true,
		},
		{
			name: "v7 cluster with annotation but ES call returns an error: propagate the error",
			args: args{
				es:       withAnnotations(esv7(), map[string]string{InitialMasterNodesAnnotation: "foo,bar"}),
				esClient: &mockZen2BootstrapESClient{zen2Bootstrapped: false, err: errors.New("err")},
			},
			wantRequeue:    false,
			wantAnnotation: true,
		},
		{
			name: "v7 cluster with annotation, bootstrap is over: remove the annotation",
			args: args{
				es:       withAnnotations(esv7(), map[string]string{InitialMasterNodesAnnotation: "foo,bar"}),
				esClient: &mockZen2BootstrapESClient{zen2Bootstrapped: true, err: nil},
			},
			wantRequeue:    false,
			wantAnnotation: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := k8s.NewFakeClient(&tt.args.es)
			requeue, err := RemoveZen2BootstrapAnnotation(context.Background(), k8sClient, tt.args.es, tt.args.esClient)
			if tt.args.esClient.(*mockZen2BootstrapESClient).err != nil { //nolint:forcetypeassert
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.wantRequeue, requeue)
			var updatedES esv1.Elasticsearch
			err = k8sClient.Get(context.Background(), k8s.ExtractNamespacedName(&tt.args.es), &updatedES)
			require.NoError(t, err)
			_, exists := updatedES.Annotations[InitialMasterNodesAnnotation]
			require.Equal(t, tt.wantAnnotation, exists)
		})
	}
}

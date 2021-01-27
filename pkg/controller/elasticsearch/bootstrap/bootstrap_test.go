// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package bootstrap

import (
	"context"
	"errors"
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
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

func notBootstrappedES() *esv1.Elasticsearch {
	return &esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec:       esv1.ElasticsearchSpec{Version: "7.3.0"},
	}
}

type fakeESClient struct {
	esclient.Client
	uuid string
	err  error
}

func (f *fakeESClient) GetClusterInfo(ctx context.Context) (esclient.Info, error) {
	return esclient.Info{ClusterUUID: f.uuid}, f.err
}

func TestReconcileClusterUUID1(t *testing.T) {
	type args struct {
		cluster     *esv1.Elasticsearch
		esClient    esclient.Client
		esReachable bool
	}
	tests := []struct {
		name           string
		args           args
		wantRequeue    bool
		wantErr        bool
		wantAnnotation string
	}{
		{
			name: "cluster already annotated, nothing to do",
			args: args{
				cluster:     bootstrappedES(),
				esReachable: true,
			},
			wantRequeue:    false,
			wantAnnotation: "uuid",
		},
		{
			name: "es not reachable yet, should requeue",
			args: args{
				cluster:     notBootstrappedES(),
				esReachable: false,
			},
			wantRequeue:    true,
			wantAnnotation: "",
		},
		{
			name: "returned uuid is empty, should requeue",
			args: args{
				cluster:     notBootstrappedES(),
				esReachable: true,
				esClient:    &fakeESClient{uuid: ""},
			},
			wantRequeue:    true,
			wantAnnotation: "",
		},
		{
			name: "returned uuid is _na_, should requeue",
			args: args{
				cluster:     notBootstrappedES(),
				esReachable: true,
				esClient:    &fakeESClient{uuid: formingClusterUUID},
			},
			wantRequeue:    true,
			wantAnnotation: "",
		},
		{
			name: "es client returns an error",
			args: args{
				cluster:     notBootstrappedES(),
				esReachable: true,
				esClient:    &fakeESClient{uuid: "", err: errors.New("error")},
			},
			wantRequeue:    true,
			wantErr:        false,
			wantAnnotation: "",
		},
		{
			name: "es client returns a uuid",
			args: args{
				cluster:     notBootstrappedES(),
				esReachable: true,
				esClient:    &fakeESClient{uuid: "abcd"},
			},
			wantRequeue:    false,
			wantErr:        false,
			wantAnnotation: "abcd",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := k8s.NewFakeClient(tt.args.cluster)
			requeue, err := ReconcileClusterUUID(context.Background(), k8sClient, tt.args.cluster, tt.args.esClient, tt.args.esReachable)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.wantRequeue, requeue)
			// get back the cluster
			var updatedCluster esv1.Elasticsearch
			err = k8sClient.Get(context.Background(), k8s.ExtractNamespacedName(tt.args.cluster), &updatedCluster)
			require.NoError(t, err)
			require.Equal(t, tt.wantAnnotation, updatedCluster.Annotations[ClusterUUIDAnnotationName])
		})
	}
}

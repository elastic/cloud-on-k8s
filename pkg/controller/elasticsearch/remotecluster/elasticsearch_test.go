// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.
package remotecluster

import (
	"context"
	"reflect"
	"testing"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_getCurrentRemoteClusters(t *testing.T) {
	type args struct {
		es esv1.Elasticsearch
	}
	tests := []struct {
		name    string
		args    args
		want    map[string]string
		wantErr bool
	}{
		{
			name: "Read from a nil annotation should be ok",
			args: args{es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "ns1",
					Namespace:   "es1",
					Annotations: map[string]string{},
				},
			}},
			want: nil,
		},
		{
			name: "Decode annotation into a list of remote cluster",
			args: args{es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "ns1",
					Namespace:   "es1",
					Annotations: map[string]string{"elasticsearch.k8s.elastic.co/remote-clusters": `[{"name":"ns2-cluster-2","configHash":"3795207740"}]`},
				},
			}},
			want: map[string]string{
				"ns2-cluster-2": "3795207740",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getCurrentRemoteClusters(tt.args.es)
			if (err != nil) != tt.wantErr {
				t.Errorf("getCurrentRemoteClusters() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getCurrentRemoteClusters() = %v, want %v", got, tt.want)
			}
		})
	}
}

type fakeESClient struct {
	esclient.Client
	settings esclient.Settings
	called   bool
}

func (f *fakeESClient) UpdateSettings(_ context.Context, settings esclient.Settings) error {
	f.settings = settings
	f.called = true
	return nil
}
func newEsWithRemoteClusters(
	esNamespace, esName string,
	annotations map[string]string,
	remoteClusters ...esv1.K8sLocalRemoteCluster,
) *esv1.Elasticsearch {
	return &esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:        esName,
			Namespace:   esNamespace,
			Annotations: annotations,
		},
		Spec: esv1.ElasticsearchSpec{
			RemoteClusters: esv1.RemoteClusters{
				K8sLocal: remoteClusters,
			},
		},
	}
}

func TestUpdateRemoteCluster(t *testing.T) {
	type args struct {
		esClient *fakeESClient
		es       *esv1.Elasticsearch
	}
	tests := []struct {
		name         string
		args         args
		wantErr      bool
		wantEsCalled bool
		wantSettings esclient.Settings
	}{
		{
			name: "Create a new remote cluster",
			args: args{
				esClient: &fakeESClient{},
				es: newEsWithRemoteClusters(
					"ns1",
					"es1",
					nil,
					esv1.K8sLocalRemoteCluster{
						ElasticsearchRef: commonv1.ObjectSelector{
							Name:      "es2",
							Namespace: "ns2",
						},
					}),
			},
			wantEsCalled: true,
			wantSettings: esclient.Settings{
				PersistentSettings: &esclient.SettingsGroup{
					Cluster: esclient.Cluster{
						RemoteClusters: map[string]esclient.RemoteCluster{
							"ns2-es2": {Seeds: []string{"es2-es-transport.ns2.svc:9300"}},
						},
					},
				},
			},
		},
		{
			name: "Create a new remote cluster with no namespace",
			args: args{
				esClient: &fakeESClient{},
				es: newEsWithRemoteClusters(
					"ns1",
					"es1",
					nil,
					esv1.K8sLocalRemoteCluster{
						ElasticsearchRef: commonv1.ObjectSelector{
							Name: "es2",
						},
					}),
			},
			wantEsCalled: true,
			wantSettings: esclient.Settings{
				PersistentSettings: &esclient.SettingsGroup{
					Cluster: esclient.Cluster{
						RemoteClusters: map[string]esclient.RemoteCluster{
							"ns1-es2": {Seeds: []string{"es2-es-transport.ns1.svc:9300"}},
						},
					},
				},
			},
		},
		{
			name: "Remote cluster already exists, do not make an API call",
			args: args{
				esClient: &fakeESClient{},
				es: newEsWithRemoteClusters(
					"ns1",
					"es1",
					map[string]string{
						"elasticsearch.k8s.elastic.co/remote-clusters": `[{"name":"ns1-es2","configHash":"1038652962"}]`,
					},
					esv1.K8sLocalRemoteCluster{
						ElasticsearchRef: commonv1.ObjectSelector{
							Name: "es2",
						},
					}),
			},
			wantEsCalled: false,
		},
		{
			name: "Remote cluster already exists but has been updated, we should make an API call",
			args: args{
				esClient: &fakeESClient{},
				es: newEsWithRemoteClusters(
					"ns1",
					"es1",
					map[string]string{
						"elasticsearch.k8s.elastic.co/remote-clusters": `[{"name":"ns1-es2","configHash":"8851644973"}]`,
					},
					esv1.K8sLocalRemoteCluster{
						ElasticsearchRef: commonv1.ObjectSelector{
							Name: "es2",
						},
					}),
			},
			wantEsCalled: true,
			wantSettings: esclient.Settings{
				PersistentSettings: &esclient.SettingsGroup{
					Cluster: esclient.Cluster{
						RemoteClusters: map[string]esclient.RemoteCluster{
							"ns1-es2": {Seeds: []string{"es2-es-transport.ns1.svc:9300"}},
						},
					},
				},
			},
		},
		{
			name: "Remove existing cluster",
			args: args{
				esClient: &fakeESClient{},
				es: newEsWithRemoteClusters(
					"ns1",
					"es1",
					map[string]string{
						"elasticsearch.k8s.elastic.co/remote-clusters": `[{"name":"to-be-deleted","configHash":"8538658922"},{"name":"ns1-es2","configHash":"1038652962"}]`,
					},
					esv1.K8sLocalRemoteCluster{
						ElasticsearchRef: commonv1.ObjectSelector{
							Name: "es2",
						},
					}),
			},
			wantEsCalled: true,
			wantSettings: esclient.Settings{
				PersistentSettings: &esclient.SettingsGroup{
					Cluster: esclient.Cluster{
						RemoteClusters: map[string]esclient.RemoteCluster{
							"to-be-deleted": {Seeds: nil},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := k8s.WrappedFakeClient(tt.args.es)
			if err := UpdateRemoteCluster(context.Background(), client, tt.args.esClient, *tt.args.es); (err != nil) != tt.wantErr {
				t.Errorf("UpdateRemoteCluster() error = %v, wantErr %v", err, tt.wantErr)
			}
			// Check the settings
			assert.Equal(t, tt.wantEsCalled, tt.args.esClient.called)
			if tt.wantEsCalled {
				assert.Equal(t, tt.wantSettings, tt.args.esClient.settings)
			}
		})
	}
}

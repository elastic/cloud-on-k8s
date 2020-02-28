// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.
package remotecluster

import (
	"context"
	"reflect"
	"testing"

	"k8s.io/client-go/tools/record"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/license"
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
			want: map[string]string{},
		},
		{
			name: "Decode annotation into a list of remote cluster",
			args: args{es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "ns1",
					Namespace:   "es1",
					Annotations: map[string]string{"elasticsearch.k8s.elastic.co/remote-clusters": `{"ns2-cluster-2":"3795207740","ns5-cluster-8":"XXXXXXXXXX"}`},
				},
			}},
			want: map[string]string{
				"ns2-cluster-2": "3795207740",
				"ns5-cluster-8": "XXXXXXXXXX",
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
	settings esclient.RemoteClustersSettings
	called   bool
}

func (f *fakeESClient) UpdateRemoteClusterSettings(_ context.Context, settings esclient.RemoteClustersSettings) error {
	f.settings = settings
	f.called = true
	return nil
}
func newEsWithRemoteClusters(
	esNamespace, esName string,
	annotations map[string]string,
	remoteClusters ...esv1.RemoteCluster,
) *esv1.Elasticsearch {
	return &esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:        esName,
			Namespace:   esNamespace,
			Annotations: annotations,
		},
		Spec: esv1.ElasticsearchSpec{
			RemoteClusters: remoteClusters,
		},
	}
}

type fakeLicenseChecker struct {
	enterpriseFeaturesEnabled bool
}

func (fakeLicenseChecker) CurrentEnterpriseLicense() (*license.EnterpriseLicense, error) {
	return nil, nil
}

func (f *fakeLicenseChecker) EnterpriseFeaturesEnabled() (bool, error) {
	return f.enterpriseFeaturesEnabled, nil
}

func (fakeLicenseChecker) Valid(_ license.EnterpriseLicense) (bool, error) {
	return true, nil
}

func TestUpdateSettings(t *testing.T) {
	type args struct {
		esClient       *fakeESClient
		es             *esv1.Elasticsearch
		licenseChecker license.Checker
	}
	tests := []struct {
		name         string
		args         args
		wantErr      bool
		wantEsCalled bool
		wantSettings esclient.RemoteClustersSettings
	}{
		{
			name: "Create a new remote cluster",
			args: args{
				esClient:       &fakeESClient{},
				licenseChecker: &fakeLicenseChecker{true},
				es: newEsWithRemoteClusters(
					"ns1",
					"es1",
					nil,
					esv1.RemoteCluster{
						Name:             "ns2-es2",
						ElasticsearchRef: commonv1.ObjectSelector{Name: "es2", Namespace: "ns2"},
					},
				),
			},
			wantEsCalled: true,
			wantSettings: esclient.RemoteClustersSettings{
				PersistentSettings: &esclient.SettingsGroup{
					Cluster: esclient.RemoteClusters{
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
				esClient:       &fakeESClient{},
				licenseChecker: &fakeLicenseChecker{true},
				es: newEsWithRemoteClusters(
					"ns1",
					"es1",
					nil,
					esv1.RemoteCluster{
						Name:             "ns1-es2",
						ElasticsearchRef: commonv1.ObjectSelector{Name: "es2"},
					}),
			},
			wantEsCalled: true,
			wantSettings: esclient.RemoteClustersSettings{
				PersistentSettings: &esclient.SettingsGroup{
					Cluster: esclient.RemoteClusters{
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
				esClient:       &fakeESClient{},
				licenseChecker: &fakeLicenseChecker{true},
				es: newEsWithRemoteClusters(
					"ns1",
					"es1",
					map[string]string{
						"elasticsearch.k8s.elastic.co/remote-clusters": `{"ns1-es2":"2221154215"}`,
					},
					esv1.RemoteCluster{
						Name:             "ns1-es2",
						ElasticsearchRef: commonv1.ObjectSelector{Name: "es2"},
					}),
			},
			wantEsCalled: false,
		},
		{
			name: "Remote cluster already exists but has been updated, we should make an API call",
			args: args{
				esClient:       &fakeESClient{},
				licenseChecker: &fakeLicenseChecker{true},
				es: newEsWithRemoteClusters(
					"ns1",
					"es1",
					map[string]string{
						"elasticsearch.k8s.elastic.co/remote-clusters": `{"ns1-es2":"8851644973"}`,
					},
					esv1.RemoteCluster{
						Name:             "ns1-es2",
						ElasticsearchRef: commonv1.ObjectSelector{Name: "es2"},
					}),
			},
			wantEsCalled: true,
			wantSettings: esclient.RemoteClustersSettings{
				PersistentSettings: &esclient.SettingsGroup{
					Cluster: esclient.RemoteClusters{
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
				esClient:       &fakeESClient{},
				licenseChecker: &fakeLicenseChecker{true},
				es: newEsWithRemoteClusters(
					"ns1",
					"es1",
					map[string]string{
						"elasticsearch.k8s.elastic.co/remote-clusters": `{"to-be-deleted":"8538658922","ns1-es2":"2221154215"}`,
					},
					esv1.RemoteCluster{
						Name:             "ns1-es2",
						ElasticsearchRef: commonv1.ObjectSelector{Name: "es2"},
					}),
			},
			wantEsCalled: true,
			wantSettings: esclient.RemoteClustersSettings{
				PersistentSettings: &esclient.SettingsGroup{
					Cluster: esclient.RemoteClusters{
						RemoteClusters: map[string]esclient.RemoteCluster{
							"to-be-deleted": {Seeds: nil},
						},
					},
				},
			},
		},
		{
			name: "No valid license to create a new remote cluster",
			args: args{
				esClient:       &fakeESClient{},
				licenseChecker: &fakeLicenseChecker{false},
				es: newEsWithRemoteClusters(
					"ns1",
					"es1",
					nil,
					esv1.RemoteCluster{
						Name:             "es2-ns2",
						ElasticsearchRef: commonv1.ObjectSelector{Namespace: "ns2", Name: "es2"},
					}),
			},
			wantEsCalled: false,
		},
		{
			name: "Multiple changes in one call: remote cluster already exists but has been updated, one is added and a last one is removed.",
			args: args{
				esClient:       &fakeESClient{},
				licenseChecker: &fakeLicenseChecker{true},
				es: newEsWithRemoteClusters(
					"ns1",
					"es1",
					map[string]string{
						"elasticsearch.k8s.elastic.co/remote-clusters": `{"ns1-es2":"8851644973","ns1-es5":"8851644973"}`,
					},
					esv1.RemoteCluster{
						Name:             "ns1-es2",
						ElasticsearchRef: commonv1.ObjectSelector{Name: "es2"},
					},
					esv1.RemoteCluster{
						Name:             "ns1-es4",
						ElasticsearchRef: commonv1.ObjectSelector{Name: "es4"},
					},
				),
			},
			wantEsCalled: true,
			wantSettings: esclient.RemoteClustersSettings{
				PersistentSettings: &esclient.SettingsGroup{
					Cluster: esclient.RemoteClusters{
						RemoteClusters: map[string]esclient.RemoteCluster{
							"ns1-es2": {Seeds: []string{"es2-es-transport.ns1.svc:9300"}},
							"ns1-es5": {Seeds: nil},
							"ns1-es4": {Seeds: []string{"es4-es-transport.ns1.svc:9300"}},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := k8s.WrappedFakeClient(tt.args.es)
			if err := UpdateSettings(
				context.Background(),
				client,
				tt.args.esClient,
				record.NewFakeRecorder(100),
				tt.args.licenseChecker,
				*tt.args.es,
			); (err != nil) != tt.wantErr {
				t.Errorf("UpdateRemoteClusterSettings() error = %v, wantErr %v", err, tt.wantErr)
			}
			// Check the settings
			assert.Equal(t, tt.wantEsCalled, tt.args.esClient.called)
			if tt.wantEsCalled {
				assert.Equal(t, tt.wantSettings, tt.args.esClient.settings)
			}
		})
	}
}

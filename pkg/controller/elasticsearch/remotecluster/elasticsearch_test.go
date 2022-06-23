// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.
package remotecluster

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/license"
	esclient "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func Test_getCurrentRemoteClusters(t *testing.T) {
	type args struct {
		es esv1.Elasticsearch
	}
	tests := []struct {
		name    string
		args    args
		want    map[string]struct{}
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
			want: map[string]struct{}{},
		},
		{
			name: "Read from an empty annotation",
			args: args{es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "ns1",
					Namespace:   "es1",
					Annotations: map[string]string{ManagedRemoteClustersAnnotationName: ""},
				},
			}},
			want: map[string]struct{}{},
		},
		{
			name: "Decode annotation into a list of remote cluster",
			args: args{es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "ns1",
					Namespace:   "es1",
					Annotations: map[string]string{"elasticsearch.k8s.elastic.co/managed-remote-clusters": `ns2-cluster-2,ns5-cluster-8`},
				},
			}},
			want: map[string]struct{}{
				"ns2-cluster-2": {},
				"ns5-cluster-8": {},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getRemoteClustersInAnnotation(tt.args.es)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getRemoteClustersInAnnotation() = %v, want %v", got, tt.want)
			}
		})
	}
}

type fakeESClient struct {
	esclient.Client
	existingSettings, updatedSettings esclient.RemoteClustersSettings
	getRemoteClusterSettingsCalled    bool
	updateRemoteClusterSettingsCalled bool
}

func (f *fakeESClient) GetRemoteClusterSettings(_ context.Context) (esclient.RemoteClustersSettings, error) {
	f.getRemoteClusterSettingsCalled = true
	return f.existingSettings, nil
}

func (f *fakeESClient) UpdateRemoteClusterSettings(_ context.Context, settings esclient.RemoteClustersSettings) error {
	f.updatedSettings = settings
	f.updateRemoteClusterSettingsCalled = true
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

func TestUpdateSettings(t *testing.T) {
	emptySettings := esclient.RemoteClustersSettings{PersistentSettings: &esclient.SettingsGroup{}}
	type args struct {
		esClient       *fakeESClient
		es             *esv1.Elasticsearch
		licenseChecker license.Checker
	}
	tests := []struct {
		name                                  string
		args                                  args
		wantAnnotation                        string
		wantRequeue                           bool
		wantErr                               bool
		wantGetRemoteClusterSettingsCalled    bool
		wantUpdateRemoteClusterSettingsCalled bool
		wantSettings                          esclient.RemoteClustersSettings
	}{
		{
			name: "Nothing to create, nothing to delete",
			args: args{
				esClient:       &fakeESClient{existingSettings: emptySettings},
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: true},
				es: newEsWithRemoteClusters(
					"ns1",
					"es1",
					nil,
				),
			},
			wantRequeue:                           false,
			wantGetRemoteClusterSettingsCalled:    false,
			wantUpdateRemoteClusterSettingsCalled: false,
		},
		{
			name: "Empty annotation",
			args: args{
				esClient:       &fakeESClient{existingSettings: emptySettings},
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: true},
				es: newEsWithRemoteClusters(
					"ns1",
					"es1",
					map[string]string{"foo": "bar", ManagedRemoteClustersAnnotationName: ""},
				),
			},
			wantRequeue:                           false,
			wantGetRemoteClusterSettingsCalled:    true,
			wantUpdateRemoteClusterSettingsCalled: false,
		},
		{
			name: "Outdated annotation should be removed",
			args: args{
				esClient:       &fakeESClient{existingSettings: emptySettings},
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: true},
				es: newEsWithRemoteClusters(
					"ns1",
					"es1",
					map[string]string{ManagedRemoteClustersAnnotationName: "ns2-es2"},
				),
			},
			wantRequeue:                           false,
			wantGetRemoteClusterSettingsCalled:    true,
			wantUpdateRemoteClusterSettingsCalled: false,
		},
		{
			name: "Create a new remote cluster",
			args: args{
				esClient:       &fakeESClient{existingSettings: emptySettings},
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: true},
				es: newEsWithRemoteClusters(
					"ns1",
					"es1",
					nil,
					esv1.RemoteCluster{
						Name:             "ns2-es2",
						ElasticsearchRef: commonv1.LocalObjectSelector{Name: "es2", Namespace: "ns2"},
					},
				),
			},
			wantAnnotation:                        "ns2-es2",
			wantGetRemoteClusterSettingsCalled:    true,
			wantUpdateRemoteClusterSettingsCalled: true,
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
				esClient: &fakeESClient{
					existingSettings: esclient.RemoteClustersSettings{PersistentSettings: &esclient.SettingsGroup{}},
				},
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: true},
				es: newEsWithRemoteClusters(
					"ns1",
					"es1",
					nil,
					esv1.RemoteCluster{
						Name:             "ns1-es2",
						ElasticsearchRef: commonv1.LocalObjectSelector{Name: "es2"},
					}),
			},
			wantGetRemoteClusterSettingsCalled:    true,
			wantUpdateRemoteClusterSettingsCalled: true,
			wantAnnotation:                        "ns1-es2",
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
			name: "Custom remote cluster added by user should not be deleted",
			args: args{
				esClient: &fakeESClient{
					existingSettings: esclient.RemoteClustersSettings{
						PersistentSettings: &esclient.SettingsGroup{
							Cluster: esclient.RemoteClusters{
								RemoteClusters: map[string]esclient.RemoteCluster{
									"es-custom-remote-es": {Seeds: []string{"somewhere:9300"}},
								},
							},
						},
					},
				},
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: true},
				es: newEsWithRemoteClusters(
					"ns1",
					"es1",
					map[string]string{
						"elasticsearch.k8s.elastic.co/managed-remote-clusters": `ns2-es2`,
					},
					esv1.RemoteCluster{
						Name:             "ns2-es2",
						ElasticsearchRef: commonv1.LocalObjectSelector{Name: "es2", Namespace: "ns2"},
					},
				),
			},
			wantGetRemoteClusterSettingsCalled:    true,
			wantUpdateRemoteClusterSettingsCalled: true,
			wantAnnotation:                        "ns2-es2",
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
			name: "Sync. annotation, do not delete custom remote cluster added by user",
			args: args{
				esClient: &fakeESClient{
					existingSettings: esclient.RemoteClustersSettings{
						PersistentSettings: &esclient.SettingsGroup{
							Cluster: esclient.RemoteClusters{
								RemoteClusters: map[string]esclient.RemoteCluster{
									"es-custom-remote-es": {Seeds: []string{"somewhere:9300"}},
								},
							},
						},
					},
				},
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: true},
				es: newEsWithRemoteClusters(
					"ns1",
					"es1",
					map[string]string{
						"elasticsearch.k8s.elastic.co/managed-remote-clusters": `ns1-es2,ns2-es2"`, // ns1-es2 should be removed from the annotation
					},
					esv1.RemoteCluster{
						Name:             "ns2-es2",
						ElasticsearchRef: commonv1.LocalObjectSelector{Name: "es2", Namespace: "ns2"},
					},
				),
			},
			wantRequeue:                           false,
			wantAnnotation:                        "ns2-es2",
			wantGetRemoteClusterSettingsCalled:    true,
			wantUpdateRemoteClusterSettingsCalled: true,
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
			name: "Remote clusters already exists, we should still make an API call",
			args: args{
				esClient: &fakeESClient{
					existingSettings: esclient.RemoteClustersSettings{
						PersistentSettings: &esclient.SettingsGroup{
							Cluster: esclient.RemoteClusters{
								RemoteClusters: map[string]esclient.RemoteCluster{
									"ns1-es2": {Seeds: []string{"es2-es-transport.ns1.svc:9300"}},
									"ns1-es3": {Seeds: []string{"es3-es-transport.ns1.svc:9300"}},
								},
							},
						},
					},
				},
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: true},
				es: newEsWithRemoteClusters(
					"ns1",
					"es1",
					map[string]string{
						"elasticsearch.k8s.elastic.co/managed-remote-clusters": `ns1-es2`,
					},
					esv1.RemoteCluster{
						Name:             "ns1-es2",
						ElasticsearchRef: commonv1.LocalObjectSelector{Name: "es2"},
					}, esv1.RemoteCluster{
						Name:             "ns1-es3",
						ElasticsearchRef: commonv1.LocalObjectSelector{Name: "es3"},
					}),
			},
			wantGetRemoteClusterSettingsCalled:    true,
			wantUpdateRemoteClusterSettingsCalled: true,
			wantAnnotation:                        "ns1-es2,ns1-es3",
			wantSettings: esclient.RemoteClustersSettings{
				PersistentSettings: &esclient.SettingsGroup{
					Cluster: esclient.RemoteClusters{
						RemoteClusters: map[string]esclient.RemoteCluster{
							"ns1-es2": {Seeds: []string{"es2-es-transport.ns1.svc:9300"}},
							"ns1-es3": {Seeds: []string{"es3-es-transport.ns1.svc:9300"}},
						},
					},
				},
			},
		},
		{
			name: "Remove previously managed cluster cluster",
			args: args{
				esClient: &fakeESClient{
					existingSettings: esclient.RemoteClustersSettings{
						PersistentSettings: &esclient.SettingsGroup{
							Cluster: esclient.RemoteClusters{
								RemoteClusters: map[string]esclient.RemoteCluster{
									"ns1-es2":       {Seeds: []string{"es2-es-transport.ns1.svc:9300"}},
									"to-be-deleted": {Seeds: []string{"somewhere:9300"}},
								},
							},
						},
					},
				},
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: true},
				es: newEsWithRemoteClusters(
					"ns1",
					"es1",
					map[string]string{
						"elasticsearch.k8s.elastic.co/managed-remote-clusters": `ns1-es2,to-be-deleted`,
					},
					esv1.RemoteCluster{
						Name:             "ns1-es2",
						ElasticsearchRef: commonv1.LocalObjectSelector{Name: "es2"},
					}),
			},
			wantRequeue:                           true,
			wantAnnotation:                        "ns1-es2,to-be-deleted",
			wantGetRemoteClusterSettingsCalled:    true,
			wantUpdateRemoteClusterSettingsCalled: true,
			wantSettings: esclient.RemoteClustersSettings{
				PersistentSettings: &esclient.SettingsGroup{
					Cluster: esclient.RemoteClusters{
						RemoteClusters: map[string]esclient.RemoteCluster{
							"ns1-es2":       {Seeds: []string{"es2-es-transport.ns1.svc:9300"}},
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
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: false},
				es: newEsWithRemoteClusters(
					"ns1",
					"es1",
					nil,
					esv1.RemoteCluster{
						Name:             "es2-ns2",
						ElasticsearchRef: commonv1.LocalObjectSelector{Namespace: "ns2", Name: "es2"},
					}),
			},
			wantGetRemoteClusterSettingsCalled:    false,
			wantUpdateRemoteClusterSettingsCalled: false,
		},
		{
			name: "No valid license, nothing to create, nothing to delete",
			args: args{
				esClient:       &fakeESClient{existingSettings: emptySettings},
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: false},
				es: &esv1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "es1",
						Namespace:   "ns1",
						Annotations: nil,
					},
				},
			},
			wantRequeue:                           false,
			wantGetRemoteClusterSettingsCalled:    false,
			wantUpdateRemoteClusterSettingsCalled: false,
		},
		{
			name: "Multiple changes: remote cluster already exists but has been updated, one is added and a last one is removed.",
			args: args{
				esClient: &fakeESClient{
					existingSettings: esclient.RemoteClustersSettings{
						PersistentSettings: &esclient.SettingsGroup{
							Cluster: esclient.RemoteClusters{
								RemoteClusters: map[string]esclient.RemoteCluster{
									"ns1-es2": {Seeds: []string{"somewhere:9300"}},
									"ns1-es5": {Seeds: []string{"somewhere:9300"}},
								},
							},
						},
					},
				},
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: true},
				es: newEsWithRemoteClusters(
					"ns1",
					"es1",
					map[string]string{
						"elasticsearch.k8s.elastic.co/managed-remote-clusters": `ns1-es2,ns1-es5`,
					},
					esv1.RemoteCluster{
						Name:             "ns1-es2",
						ElasticsearchRef: commonv1.LocalObjectSelector{Name: "es2"},
					},
					esv1.RemoteCluster{
						Name:             "ns1-es4",
						ElasticsearchRef: commonv1.LocalObjectSelector{Name: "es4"},
					},
				),
			},
			wantRequeue:                           true, // ns1-es5 has been deleted, requeue to sync the annotation
			wantGetRemoteClusterSettingsCalled:    true,
			wantUpdateRemoteClusterSettingsCalled: true,
			wantAnnotation:                        "ns1-es2,ns1-es4,ns1-es5",
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
			client := k8s.NewFakeClient(tt.args.es)
			shouldRequeue, err := UpdateSettings(
				context.Background(),
				client,
				tt.args.esClient,
				record.NewFakeRecorder(100),
				tt.args.licenseChecker,
				*tt.args.es,
			)
			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateRemoteClusterSettings() error = %v, wantErr %v", err, tt.wantErr)
			}
			// Check the annotation set on Elasticsearch
			es := &esv1.Elasticsearch{}
			assert.NoError(t, client.Get(context.Background(), k8s.ExtractNamespacedName(tt.args.es), es))

			gotAnnotation, annotationExists := es.Annotations["elasticsearch.k8s.elastic.co/managed-remote-clusters"]
			if tt.wantAnnotation != "" {
				assert.Equal(t, tt.wantAnnotation, gotAnnotation)
			} else {
				assert.False(t, annotationExists)
			}

			// Check the requeue result
			assert.Equal(t, tt.wantRequeue, shouldRequeue)
			assert.Equal(t, tt.wantGetRemoteClusterSettingsCalled, tt.args.esClient.getRemoteClusterSettingsCalled)
			// Check if the Cluster update settings ES API is called
			assert.Equal(t, tt.wantUpdateRemoteClusterSettingsCalled, tt.args.esClient.updateRemoteClusterSettingsCalled)
			if tt.wantUpdateRemoteClusterSettingsCalled {
				assert.Equal(t, tt.wantSettings, tt.args.esClient.updatedSettings)
			}
		})
	}
}

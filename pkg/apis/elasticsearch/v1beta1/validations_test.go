// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1beta1

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	common "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1beta1"
)

func Test_checkNodeSetNameUniqueness(t *testing.T) {
	type args struct {
		name         string
		es           *Elasticsearch
		expectErrors bool
	}
	tests := []args{
		{
			name: "several duplicate nodeSets",
			es: &Elasticsearch{
				Spec: ElasticsearchSpec{
					Version: "7.4.0",
					NodeSets: []NodeSet{
						{Name: "foo", Count: 1}, {Name: "foo", Count: 1},
						{Name: "bar", Count: 1}, {Name: "bar", Count: 1},
					},
				},
			},
			expectErrors: true,
		},
		{
			name: "good spec with 1 nodeSet",
			es: &Elasticsearch{
				Spec: ElasticsearchSpec{
					Version:  "7.4.0",
					NodeSets: []NodeSet{{Name: "foo", Count: 1}},
				},
			},
			expectErrors: false,
		},
		{
			name: "good spec with 2 nodeSets",
			es: &Elasticsearch{
				TypeMeta: metav1.TypeMeta{APIVersion: "elasticsearch.k8s.elastic.co/v1beta1"},
				Spec: ElasticsearchSpec{
					Version:  "7.4.0",
					NodeSets: []NodeSet{{Name: "foo", Count: 1}, {Name: "bar", Count: 1}},
				},
			},
			expectErrors: false,
		},
		{
			name: "duplicate nodeSet",
			es: &Elasticsearch{
				TypeMeta: metav1.TypeMeta{APIVersion: "elasticsearch.k8s.elastic.co/v1beta1"},
				Spec: ElasticsearchSpec{
					Version:  "7.4.0",
					NodeSets: []NodeSet{{Name: "foo", Count: 1}, {Name: "foo", Count: 1}},
				},
			},
			expectErrors: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := checkNodeSetNameUniqueness(tt.es)
			actualErrors := len(actual) > 0

			if tt.expectErrors != actualErrors {
				t.Errorf("failed checkNodeSetNameUniqueness(). Name: %v, actual %v, wanted: %v, value: %v", tt.name, actual, tt.expectErrors, tt.es.Spec.NodeSets)
			}
		})
	}
}

func Test_hasMaster(t *testing.T) {
	tests := []struct {
		name         string
		es           *Elasticsearch
		expectErrors bool
	}{
		{
			name:         "no topology",
			es:           es("6.8.0"),
			expectErrors: true,
		},
		{
			name: "topology but no master",
			es: &Elasticsearch{
				Spec: ElasticsearchSpec{
					Version: "7.0.0",
					NodeSets: []NodeSet{
						{
							Config: &common.Config{
								Data: map[string]interface{}{
									NodeMaster: "false",
									NodeData:   "false",
									NodeIngest: "false",
									NodeML:     "false",
								},
							},
						},
					},
				},
			},
			expectErrors: true,
		},
		{
			name: "master but zero sized",
			es: &Elasticsearch{
				Spec: ElasticsearchSpec{
					Version: "7.0.0",
					NodeSets: []NodeSet{
						{
							Config: &common.Config{
								Data: map[string]interface{}{
									NodeMaster: "true",
									NodeData:   "false",
									NodeIngest: "false",
									NodeML:     "false",
								},
							},
						},
					},
				},
			},
			expectErrors: true,
		},
		{
			name: "has master",
			es: &Elasticsearch{
				Spec: ElasticsearchSpec{
					Version: "7.0.0",
					NodeSets: []NodeSet{
						{
							Config: &common.Config{
								Data: map[string]interface{}{
									NodeMaster: "false",
									NodeData:   "true",
									NodeIngest: "false",
									NodeML:     "false",
								},
							},
							Count: 1,
						},

						{
							Config: &common.Config{
								Data: map[string]interface{}{
									NodeMaster: "true",
									NodeData:   "false",
									NodeIngest: "false",
									NodeML:     "false",
								},
							},
							Count: 1,
						},
					},
				},
			},
			expectErrors: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := hasMaster(tt.es)
			actualErrors := len(actual) > 0
			if tt.expectErrors != actualErrors {
				t.Errorf("failed hasMaster(). Name: %v, actual %v, wanted: %v, value: %v", tt.name, actual, tt.expectErrors, tt.es.Spec.NodeSets)
			}
		})
	}
}

func Test_supportedVersion(t *testing.T) {
	tests := []struct {
		name         string
		es           *Elasticsearch
		expectErrors bool
	}{
		{
			name: "unsupported minor version should fail",
			es:   es("6.0.0"),

			expectErrors: true,
		},
		{
			name:         "unsupported major should fail",
			es:           es("1.0.0"),
			expectErrors: true,
		},
		{
			name:         "supported OK",
			es:           es("6.8.0"),
			expectErrors: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := supportedVersion(tt.es)
			actualErrors := len(actual) > 0
			if tt.expectErrors != actualErrors {
				t.Errorf("failed supportedVersion(). Name: %v, actual %v, wanted: %v, value: %v", tt.name, actual, tt.expectErrors, tt.es.Spec.Version)
			}
		})
	}
}

func Test_validName(t *testing.T) {
	tests := []struct {
		name         string
		es           *Elasticsearch
		expectErrors bool
	}{
		{
			name: "name length too long",
			es: &Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "that-is-a-very-long-name-with-37chars",
				},
			},
			expectErrors: true,
		},
		{
			name: "name length OK",
			es: &Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "that-is-a-very-long-name-with-36char",
				},
			},
			expectErrors: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := validName(tt.es)
			actualErrors := len(actual) > 0
			if tt.expectErrors != actualErrors {
				t.Errorf("failed validName(). Name: %v, actual %v, wanted: %v, value: %v", tt.name, actual, tt.expectErrors, tt.es.Name)
			}
		})
	}
}

func Test_validSanIP(t *testing.T) {
	validIP := "3.4.5.6"
	validIP2 := "192.168.12.13"
	validIPv6 := "2001:db8:0:85a3:0:0:ac1f:8001"
	invalidIP := "notanip"

	tests := []struct {
		name         string
		es           *Elasticsearch
		expectErrors bool
	}{
		{
			name: "no SAN IP: OK",
			es: &Elasticsearch{
				Spec: ElasticsearchSpec{},
			},
			expectErrors: false,
		},
		{
			name: "valid SAN IPs: OK",
			es: &Elasticsearch{
				Spec: ElasticsearchSpec{
					HTTP: common.HTTPConfig{
						TLS: common.TLSOptions{
							SelfSignedCertificate: &common.SelfSignedCertificate{
								SubjectAlternativeNames: []common.SubjectAlternativeName{
									{
										IP: validIP,
									},
									{
										IP: validIP2,
									},
									{
										IP: validIPv6,
									},
								},
							},
						},
					},
				},
			},
			expectErrors: false,
		},
		{
			name: "invalid SAN IPs: NOT OK",
			es: &Elasticsearch{
				Spec: ElasticsearchSpec{
					HTTP: common.HTTPConfig{
						TLS: common.TLSOptions{
							SelfSignedCertificate: &common.SelfSignedCertificate{
								SubjectAlternativeNames: []common.SubjectAlternativeName{
									{
										IP: invalidIP,
									},
									{
										IP: validIP2,
									},
								},
							},
						},
					},
				},
			},
			expectErrors: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := validSanIP(tt.es)
			actualErrors := len(actual) > 0
			if tt.expectErrors != actualErrors {
				t.Errorf("failed validSanIP(). Name: %v, actual %v, wanted: %v, value: %v", tt.name, actual, tt.expectErrors, tt.es.Spec)
			}
		})
	}
}

func Test_pvcModified(t *testing.T) {
	current := getEsCluster()

	tests := []struct {
		name         string
		current      *Elasticsearch
		proposed     *Elasticsearch
		expectErrors bool
	}{
		{
			name:    "resize fails",
			current: current,
			proposed: &Elasticsearch{
				Spec: ElasticsearchSpec{
					Version: "7.2.0",
					NodeSets: []NodeSet{
						{
							Name: "master",
							VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
								{
									ObjectMeta: metav1.ObjectMeta{
										Name: "elasticsearch-data",
									},
									Spec: corev1.PersistentVolumeClaimSpec{
										Resources: corev1.ResourceRequirements{
											Requests: corev1.ResourceList{
												corev1.ResourceStorage: resource.MustParse("10Gi"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectErrors: true,
		},

		{
			name:    "same size accepted",
			current: current,
			proposed: &Elasticsearch{
				Spec: ElasticsearchSpec{
					Version: "7.2.0",
					NodeSets: []NodeSet{
						{
							Name: "master",
							VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
								{
									ObjectMeta: metav1.ObjectMeta{
										Name: "elasticsearch-data",
									},
									Spec: corev1.PersistentVolumeClaimSpec{
										Resources: corev1.ResourceRequirements{
											Requests: corev1.ResourceList{
												corev1.ResourceStorage: resource.MustParse("5Gi"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectErrors: false,
		},

		{
			name:    "additional PVC fails",
			current: current,
			proposed: &Elasticsearch{
				Spec: ElasticsearchSpec{
					Version: "7.2.0",
					NodeSets: []NodeSet{
						{
							Name: "master",
							VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
								{
									ObjectMeta: metav1.ObjectMeta{
										Name: "elasticsearch-data",
									},
									Spec: corev1.PersistentVolumeClaimSpec{
										Resources: corev1.ResourceRequirements{
											Requests: corev1.ResourceList{
												corev1.ResourceStorage: resource.MustParse("5Gi"),
											},
										},
									},
								},
								{
									ObjectMeta: metav1.ObjectMeta{
										Name: "elasticsearch-data1",
									},
									Spec: corev1.PersistentVolumeClaimSpec{
										Resources: corev1.ResourceRequirements{
											Requests: corev1.ResourceList{
												corev1.ResourceStorage: resource.MustParse("5Gi"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectErrors: true,
		},

		{
			name:    "name change rejected",
			current: current,
			proposed: &Elasticsearch{
				Spec: ElasticsearchSpec{
					Version: "7.2.0",
					NodeSets: []NodeSet{
						{
							Name: "master",
							VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
								{
									ObjectMeta: metav1.ObjectMeta{
										Name: "elasticsearch-data1",
									},
									Spec: corev1.PersistentVolumeClaimSpec{
										Resources: corev1.ResourceRequirements{
											Requests: corev1.ResourceList{
												corev1.ResourceStorage: resource.MustParse("5Gi"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectErrors: true,
		},

		{
			name:    "add new node set accepted",
			current: current,
			proposed: &Elasticsearch{
				Spec: ElasticsearchSpec{
					Version: "7.2.0",
					NodeSets: []NodeSet{
						{
							Name: "master",
							VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
								{
									ObjectMeta: metav1.ObjectMeta{
										Name: "elasticsearch-data",
									},
									Spec: corev1.PersistentVolumeClaimSpec{
										Resources: corev1.ResourceRequirements{
											Requests: corev1.ResourceList{
												corev1.ResourceStorage: resource.MustParse("5Gi"),
											},
										},
									},
								},
							},
						},
						{
							Name: "ingest",
							VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
								{
									ObjectMeta: metav1.ObjectMeta{
										Name: "elasticsearch-data",
									},
									Spec: corev1.PersistentVolumeClaimSpec{
										Resources: corev1.ResourceRequirements{
											Requests: corev1.ResourceList{
												corev1.ResourceStorage: resource.MustParse("10Gi"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectErrors: false,
		},

		{
			name:         "new instance accepted",
			current:      nil,
			proposed:     current,
			expectErrors: false,
		},
	}

	for _, tt := range tests {
		actual := pvcModification(tt.current, tt.proposed)
		actualErrors := len(actual) > 0
		if tt.expectErrors != actualErrors {
			t.Errorf("failed pvcModification(). Name: %v, actual %v, wanted: %v, value: %v", tt.name, actual, tt.expectErrors, tt.proposed)
		}
	}
}

func TestValidation_noDowngrades(t *testing.T) {
	tests := []struct {
		name         string
		current      *Elasticsearch
		proposed     *Elasticsearch
		expectErrors bool
	}{
		{
			name:         "no validation on create",
			current:      nil,
			proposed:     es("6.8.0"),
			expectErrors: false,
		},
		{
			name:         "prevent downgrade",
			current:      es("2.0.0"),
			proposed:     es("1.0.0"),
			expectErrors: true,
		},
		{
			name:         "allow upgrades",
			current:      es("1.0.0"),
			proposed:     es("1.2.0"),
			expectErrors: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := noDowngrades(tt.current, tt.proposed)
			actualErrors := len(actual) > 0
			if tt.expectErrors != actualErrors {
				t.Errorf("failed noDowngrades(). Name: %v, actual %v, wanted: %v, value: %v", tt.name, actual, tt.expectErrors, tt.proposed)
			}
		})
	}
}

func Test_validUpgradePath(t *testing.T) {
	tests := []struct {
		name         string
		current      *Elasticsearch
		proposed     *Elasticsearch
		expectErrors bool
	}{
		{
			name:         "new cluster accepted",
			current:      nil,
			proposed:     es("1.0.0"),
			expectErrors: false,
		},
		{
			name:     "unsupported version rejected",
			current:  es("1.0.0"),
			proposed: es("2.0.0"),

			expectErrors: true,
		},
		{
			name:         "too old version rejected",
			current:      es("6.5.0"),
			proposed:     es("7.0.0"),
			expectErrors: true,
		},
		{
			name:         "too new rejected",
			current:      es("7.0.0"),
			proposed:     es("6.5.0"),
			expectErrors: true,
		},
		{
			name:         "in range accepted",
			current:      es("6.8.0"),
			proposed:     es("7.1.0"),
			expectErrors: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := validUpgradePath(tt.current, tt.proposed)
			actualErrors := len(actual) > 0
			if tt.expectErrors != actualErrors {
				t.Errorf("failed validUpgradePath(). Name: %v, actual %v, wanted: %v, value: %v", tt.name, actual, tt.expectErrors, tt.proposed)
			}
		})
	}
}

// es returns an es fixture at a given version
func es(v string) *Elasticsearch {
	return &Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "foo",
		},
		Spec: ElasticsearchSpec{Version: v},
	}
}

// // getEsCluster returns a ES cluster test fixture
func getEsCluster() *Elasticsearch {
	return &Elasticsearch{
		Spec: ElasticsearchSpec{
			Version: "7.2.0",
			NodeSets: []NodeSet{
				{
					Name: "master",
					VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "elasticsearch-data",
							},
							Spec: corev1.PersistentVolumeClaimSpec{
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceStorage: resource.MustParse("5Gi"),
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func Test_noUnknownFields(t *testing.T) {
	var GetEsWithLastApplied = func(lastApplied string) Elasticsearch {
		return Elasticsearch{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					corev1.LastAppliedConfigAnnotation: lastApplied,
				},
			},
		}
	}

	tests := []struct {
		name         string
		es           Elasticsearch
		errorOnField string
	}{
		{
			name: "good annotation",
			es: GetEsWithLastApplied(
				`{"apiVersion":"elasticsearch.k8s.elastic.co/v1beta1","kind":"Elasticsearch"` +
					`,"metadata":{"annotations":{},"name":"quickstart","namespace":"default"},` +
					`"spec":{"nodeSets":[{"config":{"node.store.allow_mmap":false},"count":1,` +
					`"name":"default"}],"version":"7.5.1"}}`),
		},
		{
			name: "no annotation",
			es:   Elasticsearch{},
		},
		{
			name: "bad annotation",
			es: GetEsWithLastApplied(
				`{"apiVersion":"elasticsearch.k8s.elastic.co/v1beta1","kind":"Elasticsearch"` +
					`,"metadata":{"annotations":{},"name":"quickstart","namespace":"default"},` +
					`"spec":{"nodeSets":[{"config":{"node.store.allow_mmap":false},"count":1,` +
					`"name":"default","wrongthing":true}],"version":"7.5.1"}}`),
			errorOnField: "wrongthing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := noUnknownFields(&tt.es)
			actualErrors := len(actual) > 0
			expectErrors := tt.errorOnField != ""
			if expectErrors != actualErrors || (actualErrors && actual[0].Field != tt.errorOnField) {
				t.Errorf(
					"failed NoUnknownFields(). Name: %v, actual %v, wanted error on field: %v, es value: %v",
					tt.name,
					actual,
					tt.errorOnField,
					tt.es)
			}
		})
	}
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1beta1

import (
	// "fmt"
	// "reflect"
	// "strings"
	"testing"

	// "k8s.io/apimachinery/pkg/api/resource"

	// "github.com/stretchr/testify/require"
	common "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// // "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	// // estype "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	// common_name "github.com/elastic/cloud-on-k8s/pkg/controller/common/name"
	// "github.com/elastic/cloud-on-k8s/pkg/controller/common/validation"
	// "github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	// // "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
	// corev1 "k8s.io/api/core/v1"
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
				// TypeMeta: metav1.TypeMeta{APIVersion: "elasticsearch.k8s.elastic.co/v1beta1"},
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
				// TypeMeta: metav1.TypeMeta{APIVersion: "elasticsearch.k8s.elastic.co/v1beta1"},
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

// func Test_specUpdatedToBeta(t *testing.T) {
// 	type args struct {
// 		name        string
// 		es          Elasticsearch
// 		wantReason  string
// 		wantAllowed bool
// 	}
// 	tests := []args{
// 		{
// 			name: "good spec",
// 			es: Elasticsearch{
// 				TypeMeta: metav1.TypeMeta{APIVersion: "elasticsearch.k8s.elastic.co/v1beta1"},
// 				Spec: ElasticsearchSpec{
// 					Version:  "7.4.0",
// 					NodeSets: []NodeSet{{Count: 1}},
// 				},
// 			},
// 			wantAllowed: true,
// 		},
// 		{
// 			name: "nodes instead of nodeSets",
// 			es: Elasticsearch{
// 				Spec: ElasticsearchSpec{
// 					Version: "7.4.0",
// 				},
// 				TypeMeta: metav1.TypeMeta{APIVersion: "elasticsearch.k8s.elastic.co/v1beta1"},
// 			},
// 			wantReason: validationFailedMsg,
// 		},
// 		{
// 			name: "nodeCount instead of count",
// 			es: Elasticsearch{
// 				TypeMeta: metav1.TypeMeta{APIVersion: "elasticsearch.k8s.elastic.co/v1beta1"},
// 				Spec: ElasticsearchSpec{
// 					Version:  "7.4.0",
// 					NodeSets: []NodeSet{{}},
// 				},
// 			},
// 			wantReason:  validationFailedMsg,
// 			wantAllowed: false,
// 		},
// 		{
// 			name: "alpha instead of beta version",
// 			es: Elasticsearch{
// 				TypeMeta: metav1.TypeMeta{APIVersion: "elasticsearch.k8s.elastic.co/v1alpha1"},
// 				Spec: ElasticsearchSpec{
// 					Version:  "7.4.0",
// 					NodeSets: []NodeSet{{Count: 1}},
// 				},
// 			},
// 			wantReason:  validationFailedMsg,
// 			wantAllowed: false,
// 		},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			ctx, err := NewValidationContext(nil, tt.es)
// 			require.NoError(t, err)
// 			got := specUpdatedToBeta(*ctx)
// 			if got.Allowed != tt.wantAllowed {
// 				t.Errorf("specUpdatedToBeta() = %v, want %v", got.Allowed, tt.wantAllowed)
// 			}
// 			if !strings.Contains(got.Reason, tt.wantReason) {
// 				t.Errorf("specUpdatedToBeta() = %v, want %v", got.Reason, tt.wantReason)
// 			}
// 		})
// 	}
// }

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

func Test_noBlacklistedSettings(t *testing.T) {
	tests := []struct {
		name         string
		es           *Elasticsearch
		expectErrors bool
	}{

		{
			name:         "no settings OK",
			es:           es("7.0.0"),
			expectErrors: false,
		},
		{
			name: "enforce blacklist FAIL",
			es: &Elasticsearch{
				Spec: ElasticsearchSpec{
					Version: "7.0.0",
					NodeSets: []NodeSet{
						{
							Config: &common.Config{
								Data: map[string]interface{}{
									ClusterInitialMasterNodes: "foo",
								},
							},
							Count: 1,
						},
					},
				},
			},
			expectErrors: true,
		},
		{
			name: "enforce blacklist in multiple nodes FAIL",
			es: &Elasticsearch{
				Spec: ElasticsearchSpec{
					Version: "7.0.0",
					NodeSets: []NodeSet{
						{
							Config: &common.Config{
								Data: map[string]interface{}{
									ClusterInitialMasterNodes: "foo",
								},
							},
						},
						{
							Config: &common.Config{
								Data: map[string]interface{}{
									XPackSecurityTransportSslVerificationMode: "bar",
								},
							},
						},
					},
				},
			},
			expectErrors: true,
		},
		{
			name: "non blacklisted setting OK",
			es: &Elasticsearch{
				Spec: ElasticsearchSpec{
					Version: "7.0.0",
					NodeSets: []NodeSet{
						{
							Config: &common.Config{
								Data: map[string]interface{}{
									"node.attr.box_type": "foo",
								},
							},
						},
					},
				},
			},
			expectErrors: false,
		},
		{
			name: "non blacklisted settings with blacklisted string prefix OK",
			es: &Elasticsearch{
				Spec: ElasticsearchSpec{
					Version: "7.0.0",
					NodeSets: []NodeSet{
						{
							Config: &common.Config{
								Data: map[string]interface{}{
									XPackSecurityTransportSslCertificateAuthorities: "foo",
								},
							},
						},
					},
				},
			},
			expectErrors: false,
		},
		{
			name: "settings are canonicalized before validation",
			es: &Elasticsearch{
				Spec: ElasticsearchSpec{
					Version: "7.0.0",
					NodeSets: []NodeSet{
						{
							Config: &common.Config{
								Data: map[string]interface{}{
									"cluster": map[string]interface{}{
										"initial_master_nodes": []string{"foo", "bar"},
									},
									"node.attr.box_type": "foo",
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
			actual := noBlacklistedSettings(tt.es)
			actualErrors := len(actual) > 0
			if tt.expectErrors != actualErrors {
				t.Errorf("failed noBlacklistedSettings(). Name: %v, actual %v, wanted: %v, value: %v", tt.name, actual, tt.expectErrors, tt.es.Spec.Version)
			}
		})
	}
}

func TestValidNames(t *testing.T) {
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

// func Test_validSanIP(t *testing.T) {
// 	validIP := "3.4.5.6"
// 	validIP2 := "192.168.12.13"
// 	validIPv6 := "2001:db8:0:85a3:0:0:ac1f:8001"
// 	invalidIP := "notanip"
// 	tests := []struct {
// 		name      string
// 		esCluster Elasticsearch
// 		want      validation.Result
// 	}{
// 		{
// 			name: "no SAN IP: OK",
// 			esCluster: Elasticsearch{
// 				Spec: ElasticsearchSpec{Version: "6.8.0"},
// 			},
// 			want: validation.OK,
// 		},
// 		{
// 			name: "valid SAN IPs: OK",
// 			esCluster: Elasticsearch{
// 				Spec: ElasticsearchSpec{
// 					Version: "6.8.0",
// 					HTTP: common.HTTPConfig{
// 						TLS: common.TLSOptions{
// 							SelfSignedCertificate: &common.SelfSignedCertificate{
// 								SubjectAlternativeNames: []common.SubjectAlternativeName{
// 									{
// 										IP: validIP,
// 									},
// 									{
// 										IP: validIP2,
// 									},
// 									{
// 										IP: validIPv6,
// 									},
// 								},
// 							},
// 						},
// 					},
// 				},
// 			},
// 			want: validation.OK,
// 		},
// 		{
// 			name: "invalid SAN IPs: NOT OK",
// 			esCluster: Elasticsearch{
// 				Spec: ElasticsearchSpec{
// 					Version: "6.8.0",
// 					HTTP: common.HTTPConfig{
// 						TLS: common.TLSOptions{
// 							SelfSignedCertificate: &common.SelfSignedCertificate{
// 								SubjectAlternativeNames: []common.SubjectAlternativeName{
// 									{
// 										IP: invalidIP,
// 									},
// 									{
// 										IP: validIP2,
// 									},
// 								},
// 							},
// 						},
// 					},
// 				},
// 			},
// 			want: validation.Result{Allowed: false, Reason: "invalid SAN IP address: notanip", Error: fmt.Errorf("invalid SAN IP address: notanip")},
// 		},
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			ctx, err := NewValidationContext(nil, tt.esCluster)
// 			require.NoError(t, err)
// 			if got := validSanIP(*ctx); !reflect.DeepEqual(got, tt.want) {
// 				t.Errorf("validSanIP() = %v, want %v", got, tt.want)
// 			}
// 		})
// 	}
// }

// func Test_pvcModified(t *testing.T) {
// 	failedValidation := validation.Result{Allowed: false, Reason: pvcImmutableMsg}
// 	current := getEsCluster()
// 	tests := []struct {
// 		name     string
// 		current  *Elasticsearch
// 		proposed Elasticsearch
// 		want     validation.Result
// 	}{
// 		{
// 			name:    "resize fails",
// 			current: current,
// 			proposed: Elasticsearch{
// 				Spec: ElasticsearchSpec{
// 					Version: "7.2.0",
// 					NodeSets: []NodeSet{
// 						{
// 							Name: "master",
// 							VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
// 								{
// 									ObjectMeta: metav1.ObjectMeta{
// 										Name: "elasticsearch-data",
// 									},
// 									Spec: corev1.PersistentVolumeClaimSpec{
// 										Resources: corev1.ResourceRequirements{
// 											Requests: corev1.ResourceList{
// 												corev1.ResourceStorage: resource.MustParse("10Gi"),
// 											},
// 										},
// 									},
// 								},
// 							},
// 						},
// 					},
// 				},
// 			},
// 			want: failedValidation,
// 		},

// 		{
// 			name:    "same size accepted",
// 			current: current,
// 			proposed: Elasticsearch{
// 				Spec: ElasticsearchSpec{
// 					Version: "7.2.0",
// 					NodeSets: []NodeSet{
// 						{
// 							Name: "master",
// 							VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
// 								{
// 									ObjectMeta: metav1.ObjectMeta{
// 										Name: "elasticsearch-data",
// 									},
// 									Spec: corev1.PersistentVolumeClaimSpec{
// 										Resources: corev1.ResourceRequirements{
// 											Requests: corev1.ResourceList{
// 												corev1.ResourceStorage: resource.MustParse("5Gi"),
// 											},
// 										},
// 									},
// 								},
// 							},
// 						},
// 					},
// 				},
// 			},
// 			want: validation.OK,
// 		},

// 		{
// 			name:    "additional PVC fails",
// 			current: current,
// 			proposed: Elasticsearch{
// 				Spec: ElasticsearchSpec{
// 					Version: "7.2.0",
// 					NodeSets: []NodeSet{
// 						{
// 							Name: "master",
// 							VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
// 								{
// 									ObjectMeta: metav1.ObjectMeta{
// 										Name: "elasticsearch-data",
// 									},
// 									Spec: corev1.PersistentVolumeClaimSpec{
// 										Resources: corev1.ResourceRequirements{
// 											Requests: corev1.ResourceList{
// 												corev1.ResourceStorage: resource.MustParse("5Gi"),
// 											},
// 										},
// 									},
// 								},
// 								{
// 									ObjectMeta: metav1.ObjectMeta{
// 										Name: "elasticsearch-data1",
// 									},
// 									Spec: corev1.PersistentVolumeClaimSpec{
// 										Resources: corev1.ResourceRequirements{
// 											Requests: corev1.ResourceList{
// 												corev1.ResourceStorage: resource.MustParse("5Gi"),
// 											},
// 										},
// 									},
// 								},
// 							},
// 						},
// 					},
// 				},
// 			},
// 			want: failedValidation,
// 		},

// 		{
// 			name:    "name change rejected",
// 			current: current,
// 			proposed: Elasticsearch{
// 				Spec: ElasticsearchSpec{
// 					Version: "7.2.0",
// 					NodeSets: []NodeSet{
// 						{
// 							Name: "master",
// 							VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
// 								{
// 									ObjectMeta: metav1.ObjectMeta{
// 										Name: "elasticsearch-data1",
// 									},
// 									Spec: corev1.PersistentVolumeClaimSpec{
// 										Resources: corev1.ResourceRequirements{
// 											Requests: corev1.ResourceList{
// 												corev1.ResourceStorage: resource.MustParse("5Gi"),
// 											},
// 										},
// 									},
// 								},
// 							},
// 						},
// 					},
// 				},
// 			},
// 			want: failedValidation,
// 		},

// 		{
// 			name:    "add new node set accepted",
// 			current: current,
// 			proposed: Elasticsearch{
// 				Spec: ElasticsearchSpec{
// 					Version: "7.2.0",
// 					NodeSets: []NodeSet{
// 						{
// 							Name: "master",
// 							VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
// 								{
// 									ObjectMeta: metav1.ObjectMeta{
// 										Name: "elasticsearch-data",
// 									},
// 									Spec: corev1.PersistentVolumeClaimSpec{
// 										Resources: corev1.ResourceRequirements{
// 											Requests: corev1.ResourceList{
// 												corev1.ResourceStorage: resource.MustParse("5Gi"),
// 											},
// 										},
// 									},
// 								},
// 							},
// 						},
// 						{
// 							Name: "ingest",
// 							VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
// 								{
// 									ObjectMeta: metav1.ObjectMeta{
// 										Name: "elasticsearch-data",
// 									},
// 									Spec: corev1.PersistentVolumeClaimSpec{
// 										Resources: corev1.ResourceRequirements{
// 											Requests: corev1.ResourceList{
// 												corev1.ResourceStorage: resource.MustParse("10Gi"),
// 											},
// 										},
// 									},
// 								},
// 							},
// 						},
// 					},
// 				},
// 			},
// 			want: validation.OK,
// 		},

// 		{
// 			name:     "new instance accepted",
// 			current:  nil,
// 			proposed: *current,
// 			want:     validation.OK,
// 		},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			ctx, err := NewValidationContext(current, tt.proposed)
// 			require.NoError(t, err)
// 			require.Equal(t, tt.want, pvcModification(*ctx))
// 		})
// 	}
// }

// // getEsCluster returns a ES cluster test fixture
// func getEsCluster() *Elasticsearch {
// 	return &Elasticsearch{
// 		Spec: ElasticsearchSpec{
// 			Version: "7.2.0",
// 			NodeSets: []NodeSet{
// 				{
// 					Name: "master",
// 					VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
// 						{
// 							ObjectMeta: metav1.ObjectMeta{
// 								Name: "elasticsearch-data",
// 							},
// 							Spec: corev1.PersistentVolumeClaimSpec{
// 								Resources: corev1.ResourceRequirements{
// 									Requests: corev1.ResourceList{
// 										corev1.ResourceStorage: resource.MustParse("5Gi"),
// 									},
// 								},
// 							},
// 						},
// 					},
// 				},
// 			},
// 		},
// 	}
// }

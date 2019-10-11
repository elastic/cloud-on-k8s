// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package validation

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	common "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	estype "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	common_name "github.com/elastic/cloud-on-k8s/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/validation"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
	corev1 "k8s.io/api/core/v1"
)

func Test_checkNodeSetNameUniqueness(t *testing.T) {
	type args struct {
		name        string
		es          v1beta1.Elasticsearch
		wantReason  string
		wantAllowed bool
	}
	tests := []args{
		{
			name: "several duplicate nodeSets",
			es: v1beta1.Elasticsearch{
				TypeMeta: metav1.TypeMeta{APIVersion: "elasticsearch.k8s.elastic.co/v1beta1"},
				Spec: estype.ElasticsearchSpec{
					Version: "7.4.0",
					NodeSets: []estype.NodeSet{
						{Name: "foo", Count: 1}, {Name: "foo", Count: 1},
						{Name: "bar", Count: 1}, {Name: "bar", Count: 1},
					},
				},
			},
			wantAllowed: false,
			wantReason:  "duplicate node set(s) foo, bar",
		},
		{
			name: "good spec with 1 nodeSet",
			es: v1beta1.Elasticsearch{
				TypeMeta: metav1.TypeMeta{APIVersion: "elasticsearch.k8s.elastic.co/v1beta1"},
				Spec: estype.ElasticsearchSpec{
					Version:  "7.4.0",
					NodeSets: []estype.NodeSet{{Name: "foo", Count: 1}},
				},
			},
			wantAllowed: true,
		},
		{
			name: "good spec with 2 nodeSets",
			es: v1beta1.Elasticsearch{
				TypeMeta: metav1.TypeMeta{APIVersion: "elasticsearch.k8s.elastic.co/v1beta1"},
				Spec: estype.ElasticsearchSpec{
					Version:  "7.4.0",
					NodeSets: []estype.NodeSet{{Name: "foo", Count: 1}, {Name: "bar", Count: 1}},
				},
			},
			wantAllowed: true,
		},
		{
			name: "duplicate nodeSet",
			es: v1beta1.Elasticsearch{
				TypeMeta: metav1.TypeMeta{APIVersion: "elasticsearch.k8s.elastic.co/v1beta1"},
				Spec: estype.ElasticsearchSpec{
					Version:  "7.4.0",
					NodeSets: []estype.NodeSet{{Name: "foo", Count: 1}, {Name: "foo", Count: 1}},
				},
			},
			wantAllowed: false,
			wantReason:  validationFailedMsg,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, err := NewValidationContext(nil, tt.es)
			require.NoError(t, err)
			got := checkNodeSetNameUniqueness(*ctx)
			if got.Allowed != tt.wantAllowed {
				t.Errorf("checkNodeSetNameUniqueness() = %v, want %v", got.Allowed, tt.wantAllowed)
			}
			if !strings.Contains(got.Reason, tt.wantReason) {
				t.Errorf("checkNodeSetNameUniqueness() = %v, want %v", got.Reason, tt.wantReason)
			}
		})
	}
}

func Test_hasMaster(t *testing.T) {
	failedValidation := validation.Result{Allowed: false, Reason: masterRequiredMsg}
	type args struct {
		esCluster v1beta1.Elasticsearch
	}
	tests := []struct {
		name string
		args args
		want validation.Result
	}{
		{
			name: "no topology",
			args: args{
				esCluster: *es("6.8.0"),
			},
			want: failedValidation,
		},
		{
			name: "topology but no master",
			args: args{
				esCluster: v1beta1.Elasticsearch{
					Spec: v1beta1.ElasticsearchSpec{
						Version: "7.0.0",
						NodeSets: []v1beta1.NodeSet{
							{
								Config: &common.Config{
									Data: map[string]interface{}{
										v1beta1.NodeMaster: "false",
										v1beta1.NodeData:   "false",
										v1beta1.NodeIngest: "false",
										v1beta1.NodeML:     "false",
									},
								},
							},
						},
					},
				},
			},
			want: failedValidation,
		},
		{
			name: "master but zero sized",
			args: args{
				esCluster: v1beta1.Elasticsearch{
					Spec: v1beta1.ElasticsearchSpec{
						Version: "7.0.0",
						NodeSets: []v1beta1.NodeSet{
							{
								Config: &common.Config{
									Data: map[string]interface{}{
										v1beta1.NodeMaster: "true",
										v1beta1.NodeData:   "false",
										v1beta1.NodeIngest: "false",
										v1beta1.NodeML:     "false",
									},
								},
							},
						},
					},
				},
			},
			want: failedValidation,
		},
		{
			name: "has master",
			args: args{
				esCluster: v1beta1.Elasticsearch{
					Spec: v1beta1.ElasticsearchSpec{
						Version: "7.0.0",
						NodeSets: []v1beta1.NodeSet{
							{
								Config: &common.Config{
									Data: map[string]interface{}{
										v1beta1.NodeMaster: "true",
										v1beta1.NodeData:   "false",
										v1beta1.NodeIngest: "false",
										v1beta1.NodeML:     "false",
									},
								},
								Count: 1,
							},
						},
					},
				},
			},
			want: validation.Result{Allowed: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, err := NewValidationContext(nil, tt.args.esCluster)
			require.NoError(t, err)
			if got := hasMaster(*ctx); got != tt.want {
				t.Errorf("hasMaster() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_specUpdatedToBeta(t *testing.T) {
	type args struct {
		name        string
		es          v1beta1.Elasticsearch
		wantReason  string
		wantAllowed bool
	}
	tests := []args{
		{
			name: "good spec",
			es: v1beta1.Elasticsearch{
				TypeMeta: metav1.TypeMeta{APIVersion: "elasticsearch.k8s.elastic.co/v1beta1"},
				Spec: estype.ElasticsearchSpec{
					Version:  "7.4.0",
					NodeSets: []estype.NodeSet{{Count: 1}},
				},
			},
			wantAllowed: true,
		},
		{
			name: "nodes instead of nodeSets",
			es: v1beta1.Elasticsearch{
				Spec: estype.ElasticsearchSpec{
					Version: "7.4.0",
				},
				TypeMeta: metav1.TypeMeta{APIVersion: "elasticsearch.k8s.elastic.co/v1beta1"},
			},
			wantReason: validationFailedMsg,
		},
		{
			name: "nodeCount instead of count",
			es: v1beta1.Elasticsearch{
				TypeMeta: metav1.TypeMeta{APIVersion: "elasticsearch.k8s.elastic.co/v1beta1"},
				Spec: estype.ElasticsearchSpec{
					Version:  "7.4.0",
					NodeSets: []estype.NodeSet{{}},
				},
			},
			wantReason:  validationFailedMsg,
			wantAllowed: false,
		},
		{
			name: "alpha instead of beta version",
			es: v1beta1.Elasticsearch{
				TypeMeta: metav1.TypeMeta{APIVersion: "elasticsearch.k8s.elastic.co/v1alpha1"},
				Spec: estype.ElasticsearchSpec{
					Version:  "7.4.0",
					NodeSets: []estype.NodeSet{{Count: 1}},
				},
			},
			wantReason:  validationFailedMsg,
			wantAllowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, err := NewValidationContext(nil, tt.es)
			require.NoError(t, err)
			got := specUpdatedToBeta(*ctx)
			if got.Allowed != tt.wantAllowed {
				t.Errorf("specUpdatedToBeta() = %v, want %v", got.Allowed, tt.wantAllowed)
			}
			if !strings.Contains(got.Reason, tt.wantReason) {
				t.Errorf("specUpdatedToBeta() = %v, want %v", got.Reason, tt.wantReason)
			}
		})
	}
}

func Test_supportedVersion(t *testing.T) {
	type args struct {
		esCluster estype.Elasticsearch
	}
	tests := []struct {
		name string
		args args
		want validation.Result
	}{
		{
			name: "unsupported major version should fail",
			args: args{
				esCluster: *es("6.0.0"),
			},
			want: validation.Result{Allowed: false, Reason: unsupportedVersion(&version.Version{
				Major: 6,
				Minor: 0,
				Patch: 0,
				Label: "",
			})},
		},
		{
			name: "unsupported FAIL",
			args: args{
				esCluster: *es("1.0.0"),
			},
			want: validation.Result{Allowed: false, Reason: unsupportedVersion(&version.Version{
				Major: 1,
				Minor: 0,
				Patch: 0,
				Label: "",
			})},
		},
		{
			name: "supported OK",
			args: args{
				esCluster: *es("6.8.0"),
			},
			want: validation.OK,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, err := NewValidationContext(nil, tt.args.esCluster)
			require.NoError(t, err)
			if got := supportedVersion(*ctx); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("supportedVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_noBlacklistedSettings(t *testing.T) {
	type args struct {
		es estype.Elasticsearch
	}
	tests := []struct {
		name string
		args args
		want validation.Result
	}{
		{
			name: "no settings OK",
			args: args{
				es: *es("7.0.0"),
			},
			want: validation.OK,
		},
		{
			name: "enforce blacklist FAIL",
			args: args{
				es: estype.Elasticsearch{
					Spec: estype.ElasticsearchSpec{
						Version: "7.0.0",
						NodeSets: []estype.NodeSet{
							{
								Config: &common.Config{
									Data: map[string]interface{}{
										settings.ClusterInitialMasterNodes: "foo",
									},
								},
								Count: 1,
							},
						},
					},
				},
			},
			want: validation.Result{Allowed: false, Reason: "node[0]: cluster.initial_master_nodes is not user configurable"},
		},
		{
			name: "enforce blacklist in multiple nodes FAIL",
			args: args{
				es: estype.Elasticsearch{
					Spec: estype.ElasticsearchSpec{
						Version: "7.0.0",
						NodeSets: []estype.NodeSet{
							{
								Config: &common.Config{
									Data: map[string]interface{}{
										settings.ClusterInitialMasterNodes: "foo",
									},
								},
							},
							{
								Config: &common.Config{
									Data: map[string]interface{}{
										settings.XPackSecurityTransportSslVerificationMode: "bar",
									},
								},
							},
						},
					},
				},
			},
			want: validation.Result{
				Allowed: false,
				Reason:  "node[0]: cluster.initial_master_nodes; node[1]: xpack.security.transport.ssl.verification_mode is not user configurable",
			},
		},
		{
			name: "non blacklisted setting OK",
			args: args{
				es: estype.Elasticsearch{
					Spec: estype.ElasticsearchSpec{
						Version: "7.0.0",
						NodeSets: []estype.NodeSet{
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
			},
			want: validation.OK,
		},
		{
			name: "non blacklisted settings with blacklisted string prefix OK",
			args: args{
				es: estype.Elasticsearch{
					Spec: estype.ElasticsearchSpec{
						Version: "7.0.0",
						NodeSets: []estype.NodeSet{
							{
								Config: &common.Config{
									Data: map[string]interface{}{
										settings.XPackSecurityTransportSslCertificateAuthorities: "foo",
									},
								},
							},
						},
					},
				},
			},
			want: validation.OK,
		},
		{
			name: "settings are canonicalized before validation",
			args: args{
				es: estype.Elasticsearch{
					Spec: estype.ElasticsearchSpec{
						Version: "7.0.0",
						NodeSets: []estype.NodeSet{
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
			},
			want: validation.Result{Allowed: false, Reason: "node[0]: cluster.initial_master_nodes is not user configurable"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, err := NewValidationContext(nil, tt.args.es)
			require.NoError(t, err)
			if got := noBlacklistedSettings(*ctx); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("noBlacklistedSettings() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidNames(t *testing.T) {
	type args struct {
		esCluster estype.Elasticsearch
	}
	tests := []struct {
		name string
		args args
		want validation.Result
	}{
		{
			name: "name length too long",
			args: args{
				esCluster: estype.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "that-is-a-very-long-name-with-37chars",
					},
					Spec: estype.ElasticsearchSpec{Version: "6.8.0"},
				},
			},
			want: validation.Result{
				Allowed: false,
				Reason:  invalidName(fmt.Errorf("name exceeds maximum allowed length of %d", common_name.MaxResourceNameLength)),
			},
		},
		{
			name: "name length OK",
			args: args{
				esCluster: estype.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "that-is-a-very-long-name-with-36char",
					},
					Spec: estype.ElasticsearchSpec{Version: "6.8.0"},
				},
			},
			want: validation.OK,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, err := NewValidationContext(nil, tt.args.esCluster)
			require.NoError(t, err)
			if got := validName(*ctx); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("supportedVersion() = %v, want %v", got, tt.want)
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
		name      string
		esCluster estype.Elasticsearch
		want      validation.Result
	}{
		{
			name: "no SAN IP: OK",
			esCluster: estype.Elasticsearch{
				Spec: estype.ElasticsearchSpec{Version: "6.8.0"},
			},
			want: validation.OK,
		},
		{
			name: "valid SAN IPs: OK",
			esCluster: estype.Elasticsearch{
				Spec: estype.ElasticsearchSpec{
					Version: "6.8.0",
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
			want: validation.OK,
		},
		{
			name: "invalid SAN IPs: NOT OK",
			esCluster: estype.Elasticsearch{
				Spec: estype.ElasticsearchSpec{
					Version: "6.8.0",
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
			want: validation.Result{Allowed: false, Reason: "invalid SAN IP address: notanip", Error: fmt.Errorf("invalid SAN IP address: notanip")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, err := NewValidationContext(nil, tt.esCluster)
			require.NoError(t, err)
			if got := validSanIP(*ctx); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("validSanIP() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_pvcModified(t *testing.T) {
	failedValidation := validation.Result{Allowed: false, Reason: pvcImmutableMsg}
	current := getEsCluster()
	tests := []struct {
		name     string
		current  *v1beta1.Elasticsearch
		proposed v1beta1.Elasticsearch
		want     validation.Result
	}{
		{
			name:    "resize fails",
			current: current,
			proposed: v1beta1.Elasticsearch{
				Spec: v1beta1.ElasticsearchSpec{
					Version: "7.2.0",
					NodeSets: []v1beta1.NodeSet{
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
			want: failedValidation,
		},

		{
			name:    "same size accepted",
			current: current,
			proposed: v1beta1.Elasticsearch{
				Spec: v1beta1.ElasticsearchSpec{
					Version: "7.2.0",
					NodeSets: []v1beta1.NodeSet{
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
			want: validation.OK,
		},

		{
			name:    "additional PVC fails",
			current: current,
			proposed: v1beta1.Elasticsearch{
				Spec: v1beta1.ElasticsearchSpec{
					Version: "7.2.0",
					NodeSets: []v1beta1.NodeSet{
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
			want: failedValidation,
		},

		{
			name:    "name change rejected",
			current: current,
			proposed: v1beta1.Elasticsearch{
				Spec: v1beta1.ElasticsearchSpec{
					Version: "7.2.0",
					NodeSets: []v1beta1.NodeSet{
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
			want: failedValidation,
		},

		{
			name:    "add new node set accepted",
			current: current,
			proposed: v1beta1.Elasticsearch{
				Spec: v1beta1.ElasticsearchSpec{
					Version: "7.2.0",
					NodeSets: []v1beta1.NodeSet{
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
			want: validation.OK,
		},

		{
			name:     "new instance accepted",
			current:  nil,
			proposed: *current,
			want:     validation.OK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, err := NewValidationContext(current, tt.proposed)
			require.NoError(t, err)
			require.Equal(t, tt.want, pvcModification(*ctx))
		})
	}
}

// getEsCluster returns a ES cluster test fixture
func getEsCluster() *v1beta1.Elasticsearch {
	return &v1beta1.Elasticsearch{
		Spec: v1beta1.ElasticsearchSpec{
			Version: "7.2.0",
			NodeSets: []v1beta1.NodeSet{
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

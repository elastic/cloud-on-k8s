// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func Test_checkNodeSetNameUniqueness(t *testing.T) {
	type args struct {
		name         string
		es           esv1.Elasticsearch
		expectErrors bool
	}
	tests := []args{
		{
			name: "several duplicate nodeSets",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.4.0",
					NodeSets: []esv1.NodeSet{
						{Name: "foo", Count: 1},
						{Name: "foo", Count: 1},
						{Name: "bar", Count: 1},
						{Name: "bar", Count: 1},
					},
				},
			},
			expectErrors: true,
		},
		{
			name: "good spec with 1 nodeSet",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version:  "7.4.0",
					NodeSets: []esv1.NodeSet{{Name: "foo", Count: 1}},
				},
			},
			expectErrors: false,
		},
		{
			name: "good spec with 2 nodeSets",
			es: esv1.Elasticsearch{
				TypeMeta: metav1.TypeMeta{APIVersion: "elasticsearch.k8s.elastic.co/v1"},
				Spec: esv1.ElasticsearchSpec{
					Version:  "7.4.0",
					NodeSets: []esv1.NodeSet{{Name: "foo", Count: 1}, {Name: "bar", Count: 1}},
				},
			},
			expectErrors: false,
		},
		{
			name: "duplicate nodeSet",
			es: esv1.Elasticsearch{
				TypeMeta: metav1.TypeMeta{APIVersion: "elasticsearch.k8s.elastic.co/v1"},
				Spec: esv1.ElasticsearchSpec{
					Version:  "7.4.0",
					NodeSets: []esv1.NodeSet{{Name: "foo", Count: 1}, {Name: "foo", Count: 1}},
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

func Test_hasCorrectNodeRoles(t *testing.T) {
	type m map[string]any

	esWithRoles := func(version string, count int32, nodeSetRoles ...m) esv1.Elasticsearch {
		x := es(version)
		for _, nsc := range nodeSetRoles {
			data := nsc
			var cfg *commonv1.Config
			if data != nil {
				cfg = &commonv1.Config{Data: data}
			}

			x.Spec.NodeSets = append(x.Spec.NodeSets, esv1.NodeSet{
				Count:  count,
				Config: cfg,
			})
		}

		return x
	}

	tests := []struct {
		name         string
		es           esv1.Elasticsearch
		expectErrors bool
	}{
		{
			name:         "no topology",
			es:           esWithRoles("7.0.0", 1),
			expectErrors: true,
		},
		{
			name:         "one nodeset with no config",
			es:           esWithRoles("7.6.0", 1, nil),
			expectErrors: false,
		},
		{
			name:         "no master defined (node attributes)",
			es:           esWithRoles("7.6.0", 1, m{esv1.NodeMaster: "false", esv1.NodeData: "true"}, m{esv1.NodeMaster: "true", esv1.NodeVotingOnly: "true"}),
			expectErrors: true,
		},
		{
			name:         "no master defined (node roles)",
			es:           esWithRoles("7.9.0", 1, m{esv1.NodeRoles: []esv1.NodeRole{esv1.DataRole}}, m{esv1.NodeRoles: []esv1.NodeRole{esv1.MasterRole, esv1.VotingOnlyRole}}),
			expectErrors: true,
		},
		{
			name:         "zero master nodes (node attributes)",
			es:           esWithRoles("7.6.0", 0, m{esv1.NodeMaster: "true", esv1.NodeData: "true"}, m{esv1.NodeData: "true"}),
			expectErrors: true,
		},
		{
			name:         "zero master nodes (node roles)",
			es:           esWithRoles("7.9.0", 0, m{esv1.NodeRoles: []esv1.NodeRole{esv1.MasterRole, esv1.DataRole}}, m{esv1.NodeRoles: []esv1.NodeRole{esv1.DataRole}}),
			expectErrors: true,
		},
		{
			name:         "mixed node attributes and node roles",
			es:           esWithRoles("7.9.0", 1, m{esv1.NodeMaster: "true", esv1.NodeRoles: []esv1.NodeRole{esv1.DataRole}}, m{esv1.NodeRoles: []esv1.NodeRole{esv1.DataRole, esv1.TransformRole}}),
			expectErrors: true,
		},
		{
			name:         "node roles on older version",
			es:           esWithRoles("7.6.0", 1, m{esv1.NodeRoles: []esv1.NodeRole{esv1.MasterRole}}, m{esv1.NodeRoles: []esv1.NodeRole{esv1.DataRole}}),
			expectErrors: true,
		},
		{
			name: "valid configuration (node attributes)",
			es:   esWithRoles("7.6.0", 3, m{esv1.NodeMaster: "true", esv1.NodeData: "true"}, m{esv1.NodeData: "true"}),
		},
		{
			name: "valid configuration (node roles)",
			es:   esWithRoles("7.9.0", 4, m{esv1.NodeRoles: []esv1.NodeRole{esv1.MasterRole, esv1.DataRole}}, m{esv1.NodeRoles: []esv1.NodeRole{esv1.DataRole}}, m{esv1.NodeRoles: []esv1.NodeRole{esv1.RemoteClusterClientRole}}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasCorrectNodeRoles(tt.es)
			hasErrors := len(result) > 0
			if tt.expectErrors != hasErrors {
				t.Errorf("expectedErrors=%t hasErrors=%t result=%+v", tt.expectErrors, hasErrors, result)
			}
		})
	}
}

func Test_supportedVersion(t *testing.T) {
	tests := []struct {
		name         string
		es           esv1.Elasticsearch
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
			es:           es("7.17.0"),
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

func Test_supportsRemoteClusterUsingAPIKey(t *testing.T) {
	tests := []struct {
		name         string
		es           esv1.Elasticsearch
		expectErrors bool
	}{
		{
			name: "no remote cluster settings that relies on API keys",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "foo",
				},
				Spec: esv1.ElasticsearchSpec{Version: "7.0.0"},
			},
			expectErrors: false,
		},
		{
			name: "some remote cluster API keys before required version",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "foo"},
				Spec: esv1.ElasticsearchSpec{
					Version: "8.9.99",
					RemoteClusters: []esv1.RemoteCluster{
						{
							Name:   "bar",
							APIKey: &esv1.RemoteClusterAPIKey{},
						},
					},
				},
			},
			expectErrors: true,
		},
		{
			name: "some remote cluster API keys with min required version",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "foo"},
				Spec: esv1.ElasticsearchSpec{
					Version: "8.10.0",
					RemoteClusters: []esv1.RemoteCluster{
						{
							Name:   "bar",
							APIKey: &esv1.RemoteClusterAPIKey{},
						},
					},
				},
			},
			expectErrors: false,
		},
		{
			name: "remote cluster without API keys before required version",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "foo"},
				Spec: esv1.ElasticsearchSpec{
					Version: "8.9.99",
					RemoteClusters: []esv1.RemoteCluster{
						{
							Name: "bar",
						},
					},
				},
			},
			expectErrors: false,
		},
		{
			name: "remote cluster server enabled with min required version",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "foo"},
				Spec: esv1.ElasticsearchSpec{
					Version:             "8.10.0",
					RemoteClusterServer: esv1.RemoteClusterServer{Enabled: true},
				},
			},
			expectErrors: false,
		},
		{
			name: "remote cluster server enabled before min required version",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "foo"},
				Spec: esv1.ElasticsearchSpec{
					Version:             "8.9.99",
					RemoteClusterServer: esv1.RemoteClusterServer{Enabled: true},
				},
			},
			expectErrors: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := supportsRemoteClusterUsingAPIKey(tt.es)
			actualErrors := len(actual) > 0
			if tt.expectErrors != actualErrors {
				t.Errorf("failed supportsRemoteClusterUsingAPIKey(). Name: %v, actual %v, wanted: %v, value: %v", tt.name, actual, tt.expectErrors, tt.es.Spec.Version)
			}
		})
	}
}

func Test_validName(t *testing.T) {
	tests := []struct {
		name         string
		es           esv1.Elasticsearch
		expectErrors bool
	}{
		{
			name: "name length too long",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "that-is-a-very-long-name-with-37chars",
				},
			},
			expectErrors: true,
		},
		{
			name: "name length OK",
			es: esv1.Elasticsearch{
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
		es           esv1.Elasticsearch
		expectErrors bool
	}{
		{
			name: "no SAN IP: OK",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{},
			},
			expectErrors: false,
		},
		{
			name: "valid SAN IPs: OK",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					HTTP: commonv1.HTTPConfigWithClientOptions{
						TLS: commonv1.TLSWithClientOptions{
							TLSOptions: commonv1.TLSOptions{
								SelfSignedCertificate: &commonv1.SelfSignedCertificate{
									SubjectAlternativeNames: []commonv1.SubjectAlternativeName{
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
			},
			expectErrors: false,
		},
		{
			name: "invalid SAN IPs: NOT OK",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					HTTP: commonv1.HTTPConfigWithClientOptions{
						TLS: commonv1.TLSWithClientOptions{
							TLSOptions: commonv1.TLSOptions{
								SelfSignedCertificate: &commonv1.SelfSignedCertificate{
									SubjectAlternativeNames: []commonv1.SubjectAlternativeName{
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

func TestValidation_noDowngrades(t *testing.T) {
	tests := []struct {
		name         string
		current      esv1.Elasticsearch
		proposed     esv1.Elasticsearch
		expectErrors bool
	}{
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
		current      esv1.Elasticsearch
		proposed     esv1.Elasticsearch
		expectErrors bool
	}{
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
			current:      es("7.0.0"),
			proposed:     es("7.17.0"),
			expectErrors: false,
		},
		{
			name: "not yet fully upgraded rejected",
			current: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "foo",
				},
				Spec: esv1.ElasticsearchSpec{Version: "7.17.0"},
				Status: esv1.ElasticsearchStatus{
					Version: "7.16.2",
				},
			},
			proposed:     es("8.0.0"),
			expectErrors: true, // still running at least one node with 7.16.2
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

func Test_noUnknownFields(t *testing.T) {
	GetEsWithLastApplied := func(lastApplied string) esv1.Elasticsearch {
		return esv1.Elasticsearch{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					corev1.LastAppliedConfigAnnotation: lastApplied,
				},
			},
		}
	}

	tests := []struct {
		name         string
		es           esv1.Elasticsearch
		errorOnField string
	}{
		{
			name: "good annotation",
			es: GetEsWithLastApplied(
				`{"apiVersion":"elasticsearch.k8s.elastic.co/v1","kind":"Elasticsearch"` +
					`,"metadata":{"annotations":{},"name":"quickstart","namespace":"default"},` +
					`"spec":{"nodeSets":[{"config":{"node.store.allow_mmap":false},"count":1,` +
					`"name":"default"}],"version":"7.5.1"}}`),
		},
		{
			name: "no annotation",
			es:   esv1.Elasticsearch{},
		},
		{
			name: "bad annotation",
			es: GetEsWithLastApplied(
				`{"apiVersion":"elasticsearch.k8s.elastic.co/v1","kind":"Elasticsearch"` +
					`,"metadata":{"annotations":{},"name":"quickstart","namespace":"default"},` +
					`"spec":{"nodeSets":[{"config":{"node.store.allow_mmap":false},"count":1,` +
					`"name":"default","wrongthing":true}],"version":"7.5.1"}}`),
			errorOnField: "wrongthing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := noUnknownFields(tt.es)
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

func Test_validNodeLabels(t *testing.T) {
	type args struct {
		proposed          esv1.Elasticsearch
		exposedNodeLabels []string
	}
	tests := []struct {
		name           string
		args           args
		expectedFields []string
	}{
		{
			name: "Invalid node label",
			args: args{
				proposed: esv1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{esv1.DownwardNodeLabelsAnnotation: "failure-domain.beta.kubernetes.io/zone"},
					},
				},
				exposedNodeLabels: []string{"topology.kubernetes.io/*"},
			},
			expectedFields: []string{
				field.NewPath("metadata").Child("annotations", esv1.DownwardNodeLabelsAnnotation).String(),
			},
		},
		{
			name: "Valid node label",
			args: args{
				proposed: esv1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{esv1.DownwardNodeLabelsAnnotation: "failure-domain.beta.kubernetes.io/zone"},
					},
				},
				exposedNodeLabels: []string{"topology.kubernetes.io/*", "failure-domain.beta.kubernetes.io/*"},
			},
		},
		{
			name: "Zone awareness default topology key allowed without exposed-node-labels",
			args: args{
				proposed: esv1.Elasticsearch{
					Spec: esv1.ElasticsearchSpec{
						NodeSets: []esv1.NodeSet{
							{
								Name:          "default",
								ZoneAwareness: &esv1.ZoneAwareness{},
							},
						},
					},
				},
			},
		},
		{
			name: "Zone awareness custom topology key allowed without exposed-node-labels",
			args: args{
				proposed: esv1.Elasticsearch{
					Spec: esv1.ElasticsearchSpec{
						NodeSets: []esv1.NodeSet{
							{
								Name: "default",
								ZoneAwareness: &esv1.ZoneAwareness{
									TopologyKey: "custom.io/rack",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Zone awareness custom topology key rejected when not in exposed-node-labels",
			args: args{
				proposed: esv1.Elasticsearch{
					Spec: esv1.ElasticsearchSpec{
						NodeSets: []esv1.NodeSet{
							{
								Name: "default",
								ZoneAwareness: &esv1.ZoneAwareness{
									TopologyKey: "custom.io/rack",
								},
							},
						},
					},
				},
				exposedNodeLabels: []string{"topology.kubernetes.io/.*"},
			},
			expectedFields: []string{
				field.NewPath("spec").Child("nodeSets").Index(0).Child("zoneAwareness", "topologyKey").String(),
			},
		},
		{
			name: "Zone awareness default topology key allowed when in exposed-node-labels",
			args: args{
				proposed: esv1.Elasticsearch{
					Spec: esv1.ElasticsearchSpec{
						NodeSets: []esv1.NodeSet{
							{
								Name:          "default",
								ZoneAwareness: &esv1.ZoneAwareness{},
							},
						},
					},
				},
				exposedNodeLabels: []string{"topology.kubernetes.io/.*"},
			},
		},
		{
			name: "Zone awareness custom topology key allowed when in exposed-node-labels",
			args: args{
				proposed: esv1.Elasticsearch{
					Spec: esv1.ElasticsearchSpec{
						NodeSets: []esv1.NodeSet{
							{
								Name: "default",
								ZoneAwareness: &esv1.ZoneAwareness{
									TopologyKey: "custom.io/rack",
								},
							},
						},
					},
				},
				exposedNodeLabels: []string{"custom.io/.*"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exposedNodeLabels, err := NewExposedNodeLabels(tt.args.exposedNodeLabels)
			assert.NoError(t, err)
			actual := validNodeLabels(tt.args.proposed, exposedNodeLabels)
			actualFields := make([]string, len(actual))
			for i, err := range actual {
				actualFields[i] = err.Field
			}
			assert.ElementsMatch(t, tt.expectedFields, actualFields)
		})
	}
}

func Test_validZoneAwarenessTopologyKeys(t *testing.T) {
	tests := []struct {
		name         string
		es           esv1.Elasticsearch
		expectErrors bool
	}{
		{
			name: "no zone awareness configured",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{Name: "a"},
						{Name: "b"},
					},
				},
			},
		},
		{
			name: "default topology key is consistent across nodesets",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{Name: "a", ZoneAwareness: &esv1.ZoneAwareness{}},
						{Name: "b", ZoneAwareness: &esv1.ZoneAwareness{TopologyKey: esv1.DefaultZoneAwarenessTopologyKey}},
					},
				},
			},
		},
		{
			name: "custom topology key is consistent across nodesets",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{Name: "a", ZoneAwareness: &esv1.ZoneAwareness{TopologyKey: "custom.io/rack"}},
						{Name: "b", ZoneAwareness: &esv1.ZoneAwareness{TopologyKey: "custom.io/rack"}},
					},
				},
			},
		},
		{
			name: "mixed default and custom topology keys are rejected even when all nodesets are explicit",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{Name: "a", ZoneAwareness: &esv1.ZoneAwareness{}},
						{Name: "b", ZoneAwareness: &esv1.ZoneAwareness{TopologyKey: "custom.io/rack"}},
					},
				},
			},
			expectErrors: true,
		},
		{
			name: "mixed custom topology keys are rejected even when all nodesets are explicit",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{Name: "a", ZoneAwareness: &esv1.ZoneAwareness{TopologyKey: "custom.io/room"}},
						{Name: "b", ZoneAwareness: &esv1.ZoneAwareness{TopologyKey: "custom.io/rack"}},
					},
				},
			},
			expectErrors: true,
		},
		{
			name: "single topology key with non-zoneAware nodeset is allowed",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{Name: "a", ZoneAwareness: &esv1.ZoneAwareness{TopologyKey: "custom.io/rack"}},
						{Name: "b"},
					},
				},
			},
		},
		{
			name: "ambiguous mixed topology keys are rejected when at least one nodeset is non-zoneAware",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{Name: "a", ZoneAwareness: &esv1.ZoneAwareness{TopologyKey: "custom.io/rack"}},
						{Name: "b", ZoneAwareness: &esv1.ZoneAwareness{TopologyKey: "custom.io/room"}},
						{Name: "c"},
					},
				},
			},
			expectErrors: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validZoneAwarenessTopologyKeys(tt.es)
			hasErrors := len(errs) > 0
			assert.Equal(t, tt.expectErrors, hasErrors)
		})
	}
}

func Test_validZoneAwarenessAffinityInCompatibility(t *testing.T) {
	requiredAffinityWithInExpression := func(key string, values []string) *corev1.Affinity {
		return &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{Key: key, Operator: corev1.NodeSelectorOpIn, Values: values},
							},
						},
					},
				},
			},
		}
	}

	topologyExprPath := func(nodeSetIndex int) string {
		return field.NewPath("spec").Child("nodeSets").Index(nodeSetIndex).Child("podTemplate", "spec", "affinity", "nodeAffinity", "requiredDuringSchedulingIgnoredDuringExecution", "nodeSelectorTerms").Index(0).Child("matchExpressions").Index(0).String()
	}

	tests := []struct {
		name           string
		es             esv1.Elasticsearch
		expectedFields []string
	}{
		{
			name: "rejects In with no intersection with configured zones",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{
							Name:          "za",
							ZoneAwareness: &esv1.ZoneAwareness{Zones: []string{"us-east-1a", "us-east-1b"}},
							PodTemplate: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Affinity: requiredAffinityWithInExpression(esv1.DefaultZoneAwarenessTopologyKey, []string{"us-east-1c"}),
								},
							},
						},
					},
				},
			},
			expectedFields: []string{topologyExprPath(0)},
		},
		{
			name: "rejects In with no intersection on custom topology key",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{
							Name: "za",
							ZoneAwareness: &esv1.ZoneAwareness{
								TopologyKey: "custom.io/rack",
								Zones:       []string{"rack-1", "rack-2"},
							},
							PodTemplate: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Affinity: requiredAffinityWithInExpression("custom.io/rack", []string{"rack-3", "rack-4"}),
								},
							},
						},
					},
				},
			},
			expectedFields: []string{topologyExprPath(0)},
		},
		{
			name: "allows In with partial intersection (user restricts to subset of zones)",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{
							Name:          "za",
							ZoneAwareness: &esv1.ZoneAwareness{Zones: []string{"us-east-1a", "us-east-1b", "us-east-1c"}},
							PodTemplate: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Affinity: requiredAffinityWithInExpression(esv1.DefaultZoneAwarenessTopologyKey, []string{"us-east-1a"}),
								},
							},
						},
					},
				},
			},
		},
		{
			name: "allows In with exact match of zones",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{
							Name:          "za",
							ZoneAwareness: &esv1.ZoneAwareness{Zones: []string{"us-east-1a", "us-east-1b"}},
							PodTemplate: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Affinity: requiredAffinityWithInExpression(esv1.DefaultZoneAwarenessTopologyKey, []string{"us-east-1a", "us-east-1b"}),
								},
							},
						},
					},
				},
			},
		},
		{
			name: "allows In on unrelated key",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{
							Name:          "za",
							ZoneAwareness: &esv1.ZoneAwareness{Zones: []string{"us-east-1a"}},
							PodTemplate: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Affinity: requiredAffinityWithInExpression("nodepool", []string{"pool-x"}),
								},
							},
						},
					},
				},
			},
		},
		{
			name: "skips nodesets without explicit zones (operator injects Exists, not In)",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{
							Name:          "za-no-zones",
							ZoneAwareness: &esv1.ZoneAwareness{},
							PodTemplate: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Affinity: requiredAffinityWithInExpression(esv1.DefaultZoneAwarenessTopologyKey, []string{"us-east-1c"}),
								},
							},
						},
					},
				},
			},
		},
		{
			name: "skips non-zone-aware nodesets even in zone-aware cluster",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{
							Name:          "za",
							ZoneAwareness: &esv1.ZoneAwareness{Zones: []string{"us-east-1a"}},
						},
						{
							Name: "plain",
							PodTemplate: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Affinity: requiredAffinityWithInExpression(esv1.DefaultZoneAwarenessTopologyKey, []string{"us-east-1c"}),
								},
							},
						},
					},
				},
			},
		},
		{
			name: "no affinity configured",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{
							Name:          "za",
							ZoneAwareness: &esv1.ZoneAwareness{Zones: []string{"us-east-1a"}},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validZoneAwarenessAffinityInCompatibility(tt.es)
			actualFields := make([]string, len(errs))
			for i, err := range errs {
				actualFields[i] = err.Field
			}
			assert.ElementsMatch(t, tt.expectedFields, actualFields)
		})
	}
}

func Test_validAssociations(t *testing.T) {
	type args struct {
		name         string
		es           esv1.Elasticsearch
		expectErrors bool
	}
	tests := []args{
		{
			name: "no monitoring ref: OK",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.14.0",
				},
			},
			expectErrors: false,
		},
		{
			name: "named stackmon metrics ref: OK",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version:    "7.14.0",
					Monitoring: commonv1.Monitoring{Metrics: commonv1.MetricsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "esmonname"}}}},
				},
			},
			expectErrors: false,
		},
		{
			name: "named stackmon metrics ref with namespace: OK",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version:    "7.14.0",
					Monitoring: commonv1.Monitoring{Metrics: commonv1.MetricsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "esmonname", Namespace: "esmonns"}}}},
				},
			},
			expectErrors: false,
		},
		{
			name: "named stackmon metrics ref with service name: OK",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version:    "7.14.0",
					Monitoring: commonv1.Monitoring{Metrics: commonv1.MetricsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "esmonname", ServiceName: "esmonsvc"}}}},
				},
			},
			expectErrors: false,
		},
		{
			name: "named stackmon metrics ref with namespace and service name: OK",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version:    "7.14.0",
					Monitoring: commonv1.Monitoring{Metrics: commonv1.MetricsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "esmonname", Namespace: "esmonns", ServiceName: "esmonsvc"}}}},
				},
			},
			expectErrors: false,
		},
		{
			name: "secret named stackmon metrics ref: OK",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version:    "7.14.0",
					Monitoring: commonv1.Monitoring{Metrics: commonv1.MetricsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{SecretName: "esmonname"}}}},
				},
			},
			expectErrors: false,
		},
		{
			name: "secret named stackmon logs ref: OK",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version:    "7.14.0",
					Monitoring: commonv1.Monitoring{Logs: commonv1.LogsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{SecretName: "esmonns"}}}},
				},
			},
			expectErrors: false,
		},
		{
			name: "multiple named stackmon refs: OK",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.14.0",
					Monitoring: commonv1.Monitoring{
						Metrics: commonv1.MetricsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "es1monname", Namespace: "esmonns1"}}},
						Logs:    commonv1.LogsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "es2monname", Namespace: "esmonns2"}}},
					},
				},
			},
			expectErrors: false,
		},
		{
			name: "multiple secret named stackmon refs: OK",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.14.0",
					Monitoring: commonv1.Monitoring{
						Metrics: commonv1.MetricsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{SecretName: "es1monname"}}},
						Logs:    commonv1.LogsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{SecretName: "es2monname"}}},
					},
				},
			},
			expectErrors: false,
		},
		{
			name: "mix secret named and named stackmon refs: OK",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.14.0",
					Monitoring: commonv1.Monitoring{
						Metrics: commonv1.MetricsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "es1monname", Namespace: "esmonns"}}},
						Logs:    commonv1.LogsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{SecretName: "es2monname"}}},
					},
				},
			},
			expectErrors: false,
		},
		{
			name: "invalid namespaced stackmon ref without name: NOK",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.14.0",
					Monitoring: commonv1.Monitoring{
						Metrics: commonv1.MetricsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{Namespace: "esmonns"}}},
					},
				},
			},
			expectErrors: true,
		},
		{
			name: "invalid service named stackmon ref without name: NOK",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.14.0",
					Monitoring: commonv1.Monitoring{
						Metrics: commonv1.MetricsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{ServiceName: "esmonsvc"}}},
					},
				},
			},
			expectErrors: true,
		},
		{
			name: "invalid secret named stackmon ref with name: NOK",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.14.0",
					Monitoring: commonv1.Monitoring{
						Metrics: commonv1.MetricsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{SecretName: "xx", Name: "es1monname"}}},
					},
				},
			},
			expectErrors: true,
		},
		{
			name: "invalid secret named stackmon ref with namespace name: NOK",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.14.0",
					Monitoring: commonv1.Monitoring{
						Logs: commonv1.LogsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{SecretName: "es2monname", Namespace: "esmonns"}}},
					},
				},
			},
			expectErrors: true,
		},
		{
			name: "invalid secret named stackmon ref with service name: NOK",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.14.0",
					Monitoring: commonv1.Monitoring{
						Logs: commonv1.LogsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{SecretName: "es2monname", ServiceName: "xx"}}},
					},
				},
			},
			expectErrors: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := validAssociations(tt.es)
			actualErrors := len(actual) > 0
			if tt.expectErrors != actualErrors {
				t.Errorf("failed validAssociations(). Name: %v, actual %v, wanted: %v", tt.name, actual, tt.expectErrors)
			}
		})
	}
}

func Test_fipsWarnings(t *testing.T) {
	tests := []struct {
		name    string
		objects []client.Object
		es      esv1.Elasticsearch
		want    field.ErrorList
	}{
		{
			name: "no fips in nodesets",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version:  "9.4.0",
					NodeSets: []esv1.NodeSet{{Name: "a", Count: 1, Config: &commonv1.Config{Data: map[string]any{"node.attr.rack": "r1"}}}},
				},
			},
			want: nil,
		},
		{
			name: "mixed fips settings below min version emits both warnings",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "9.3.0",
					NodeSets: []esv1.NodeSet{
						{Name: "a", Count: 1, Config: &commonv1.Config{Data: map[string]any{"xpack.security.fips_mode.enabled": true}}},
						{Name: "b", Count: 1, Config: &commonv1.Config{Data: map[string]any{"xpack.security.fips_mode.enabled": false}}},
					},
				},
			},
			want: field.ErrorList{
				field.Invalid(field.NewPath("spec").Child("nodeSets"), []esv1.NodeSet{
					{Name: "a", Count: 1, Config: &commonv1.Config{Data: map[string]any{"xpack.security.fips_mode.enabled": true}}},
					{Name: "b", Count: 1, Config: &commonv1.Config{Data: map[string]any{"xpack.security.fips_mode.enabled": false}}},
				}, inconsistentFIPSModeWarningMsg),
				field.Invalid(field.NewPath("spec").Child("version"), "9.3.0", fipsManagedKeystoreUnsupportedWarningMsg),
			},
		},
		{
			name: "fips enabled below min version but keystore password via envFrom secret",
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "ks-envfrom"},
					Data:       map[string][]byte{"KEYSTORE_PASSWORD_FILE": []byte("/x")},
				},
			},
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
				Spec: esv1.ElasticsearchSpec{
					Version: "9.3.0",
					NodeSets: []esv1.NodeSet{
						{
							Name:   "a",
							Count:  1,
							Config: &commonv1.Config{Data: map[string]any{"xpack.security.fips_mode.enabled": true}},
							PodTemplate: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{
										{
											Name: esv1.ElasticsearchContainerName,
											EnvFrom: []corev1.EnvFromSource{
												{SecretRef: &corev1.SecretEnvSource{
													LocalObjectReference: corev1.LocalObjectReference{Name: "ks-envfrom"},
												}},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want: nil,
		},
		{
			name: "version parse error still returns consistency warning",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "not-a-version",
					NodeSets: []esv1.NodeSet{
						{Name: "a", Count: 1, Config: &commonv1.Config{Data: map[string]any{"xpack.security.fips_mode.enabled": true}}},
						{Name: "b", Count: 1, Config: &commonv1.Config{Data: map[string]any{"xpack.security.fips_mode.enabled": false}}},
					},
				},
			},
			want: field.ErrorList{
				field.Invalid(field.NewPath("spec").Child("nodeSets"), []esv1.NodeSet{
					{Name: "a", Count: 1, Config: &commonv1.Config{Data: map[string]any{"xpack.security.fips_mode.enabled": true}}},
					{Name: "b", Count: 1, Config: &commonv1.Config{Data: map[string]any{"xpack.security.fips_mode.enabled": false}}},
				}, inconsistentFIPSModeWarningMsg),
				field.Invalid(field.NewPath("spec").Child("version"), "not-a-version", parseVersionErrMsg),
			},
		},
		{
			name: "fips enabled below min version but keystore password env set on es container",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "9.3.0",
					NodeSets: []esv1.NodeSet{
						{
							Name:   "a",
							Count:  1,
							Config: &commonv1.Config{Data: map[string]any{"xpack.security.fips_mode.enabled": true}},
							PodTemplate: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{
										{Name: esv1.ElasticsearchContainerName, Env: []corev1.EnvVar{{Name: "KEYSTORE_PASSWORD_FILE", Value: "/x"}}},
									},
								},
							},
						},
					},
				},
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.NewFakeClient(tt.objects...)
			got, err := fipsWarnings(context.Background(), c, tt.es)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_fipsWarnings_notFoundEnvFromDoesNotBlockAdmissionWarningPath(t *testing.T) {
	tests := []struct {
		name         string
		es           esv1.Elasticsearch
		wantWarnings []string
	}{
		{
			name: "fips enabled below min version and missing envFrom ref suppresses managed-keystore warning",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
				Spec: esv1.ElasticsearchSpec{
					Version: "9.3.0",
					NodeSets: []esv1.NodeSet{
						{
							Name:   "a",
							Count:  1,
							Config: &commonv1.Config{Data: map[string]any{"xpack.security.fips_mode.enabled": true}},
							PodTemplate: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{
										{
											Name: esv1.ElasticsearchContainerName,
											EnvFrom: []corev1.EnvFromSource{
												{SecretRef: &corev1.SecretEnvSource{
													LocalObjectReference: corev1.LocalObjectReference{Name: "missing-secret"},
												}},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantWarnings: nil,
		},
		{
			name: "mixed fips still emits consistency warning when envFrom ref is missing",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
				Spec: esv1.ElasticsearchSpec{
					Version: "9.3.0",
					NodeSets: []esv1.NodeSet{
						{
							Name:   "a",
							Count:  1,
							Config: &commonv1.Config{Data: map[string]any{"xpack.security.fips_mode.enabled": true}},
							PodTemplate: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{
										{
											Name: esv1.ElasticsearchContainerName,
											EnvFrom: []corev1.EnvFromSource{
												{SecretRef: &corev1.SecretEnvSource{
													LocalObjectReference: corev1.LocalObjectReference{Name: "missing-secret"},
												}},
											},
										},
									},
								},
							},
						},
						{
							Name:   "b",
							Count:  1,
							Config: &commonv1.Config{Data: map[string]any{"xpack.security.fips_mode.enabled": false}},
						},
					},
				},
			},
			wantWarnings: []string{inconsistentFIPSModeWarningMsg},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.NewFakeClient()
			got, err := fipsWarnings(context.Background(), c, tt.es)
			require.NoError(t, err)

			gotWarnings := make([]string, 0, len(got))
			for _, w := range got {
				gotWarnings = append(gotWarnings, w.Detail)
			}
			require.ElementsMatch(t, tt.wantWarnings, gotWarnings)
		})
	}
}

func Test_validateRestartTriggerWarnings(t *testing.T) {
	const clusterName = "foo"
	const clusterNamespace = "default"

	esCR := func(triggerValue string) esv1.Elasticsearch {
		cr := esv1.Elasticsearch{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterName,
				Namespace: clusterNamespace,
			},
		}
		if triggerValue != "" {
			cr.Annotations = map[string]string{
				esv1.RestartTriggerAnnotation: triggerValue,
			}
		}
		return cr
	}

	pod := func(name, triggerValue string) *corev1.Pod {
		p := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: clusterNamespace,
				Labels: map[string]string{
					label.ClusterNameLabelName: clusterName,
				},
			},
		}
		if triggerValue != "" {
			p.Annotations = map[string]string{
				esv1.RestartTriggerAnnotation: triggerValue,
			}
		}
		return p
	}

	tests := []struct {
		name        string
		oldCR       esv1.Elasticsearch
		newCR       esv1.Elasticsearch
		pods        []client.Object
		wantWarning string
	}{
		{
			name:        "no annotation on old or new: no warning",
			oldCR:       esCR(""),
			newCR:       esCR(""),
			wantWarning: "",
		},
		{
			name:        "annotation unchanged (same value): no warning",
			oldCR:       esCR("v1"),
			newCR:       esCR("v1"),
			wantWarning: "",
		},
		{
			name:        "annotation changed to a new value (both set): no warning",
			oldCR:       esCR("v1"),
			newCR:       esCR("v2"),
			wantWarning: "",
		},
		{
			name:        "annotation removed, no restart in progress: no warning",
			oldCR:       esCR("v1"),
			newCR:       esCR(""),
			pods:        []client.Object{pod("pod-0", "v1"), pod("pod-1", "v1")},
			wantWarning: "",
		},
		{
			name:        "annotation removed while restart in progress: removal warning",
			oldCR:       esCR("v1"),
			newCR:       esCR(""),
			pods:        []client.Object{pod("pod-0", "v1"), pod("pod-1", "old-value")},
			wantWarning: restartTriggerRemovedWarningMsg,
		},
		{
			name:        "annotation removed, no pods have the old value: restart in progress, removal warning",
			oldCR:       esCR("v1"),
			newCR:       esCR(""),
			pods:        []client.Object{pod("pod-0", "v0"), pod("pod-1", "v0")},
			wantWarning: restartTriggerRemovedWarningMsg,
		},
		{
			name:        "annotation set for the first time, pods have no annotation: no warning",
			oldCR:       esCR(""),
			newCR:       esCR("v1"),
			pods:        []client.Object{pod("pod-0", ""), pod("pod-1", "")},
			wantWarning: "",
		},
		{
			name:        "annotation re-added with value pods already have: unchanged warning",
			oldCR:       esCR(""),
			newCR:       esCR("v1"),
			pods:        []client.Object{pod("pod-0", "v1"), pod("pod-1", "v1")},
			wantWarning: restartTriggerUnchangedWarningMsg,
		},
		{
			name:        "annotation set from empty to value different from pods: no warning",
			oldCR:       esCR(""),
			newCR:       esCR("v2"),
			pods:        []client.Object{pod("pod-0", "v1"), pod("pod-1", "v1")},
			wantWarning: "",
		},
		{
			name:        "annotation re-added matches lexicographically largest pod value: unchanged warning",
			oldCR:       esCR(""),
			newCR:       esCR("v2"),
			pods:        []client.Object{pod("pod-0", "v1"), pod("pod-1", "v2")},
			wantWarning: restartTriggerUnchangedWarningMsg,
		},
		{
			name:        "annotation removed, no pods in cluster: no warning",
			oldCR:       esCR("v1"),
			newCR:       esCR(""),
			pods:        nil,
			wantWarning: "",
		},
		{
			name:        "annotation set from empty, no pods in cluster: no warning",
			oldCR:       esCR(""),
			newCR:       esCR("v1"),
			pods:        nil,
			wantWarning: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := make([]client.Object, len(tt.pods))
			copy(objs, tt.pods)
			client := k8s.NewFakeClient(objs...)

			got := validateRestartTriggerWarnings(context.Background(), client, tt.oldCR, tt.newCR)
			assert.Equal(t, tt.wantWarning, got)
		})
	}
}

// es returns an es fixture at a given version
func es(v string) esv1.Elasticsearch {
	return esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "foo",
		},
		Spec: esv1.ElasticsearchSpec{Version: v},
	}
}

func Test_validClientAuthentication(t *testing.T) {
	tests := []struct {
		name              string
		es                esv1.Elasticsearch
		enterpriseEnabled bool
		expectErrors      bool
	}{
		{
			name: "client authentication disabled: OK regardless of license",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "8.17.0",
					HTTP: commonv1.HTTPConfigWithClientOptions{
						TLS: commonv1.TLSWithClientOptions{
							Client: commonv1.ClientOptions{Authentication: false},
						},
					},
				},
			},
			enterpriseEnabled: false,
			expectErrors:      false,
		},
		{
			name: "client authentication enabled with enterprise license: OK",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "8.17.0",
					HTTP: commonv1.HTTPConfigWithClientOptions{
						TLS: commonv1.TLSWithClientOptions{
							Client: commonv1.ClientOptions{Authentication: true},
						},
					},
				},
			},
			enterpriseEnabled: true,
			expectErrors:      false,
		},
		{
			name: "client authentication enabled without enterprise license: NOK",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "8.17.0",
					HTTP: commonv1.HTTPConfigWithClientOptions{
						TLS: commonv1.TLSWithClientOptions{
							Client: commonv1.ClientOptions{Authentication: true},
						},
					},
				},
			},
			enterpriseEnabled: false,
			expectErrors:      true,
		},
		{
			name: "client authentication enabled with TLS disabled: NOK",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "8.17.0",
					HTTP: commonv1.HTTPConfigWithClientOptions{
						TLS: commonv1.TLSWithClientOptions{
							TLSOptions: commonv1.TLSOptions{
								SelfSignedCertificate: &commonv1.SelfSignedCertificate{Disabled: true},
							},
							Client: commonv1.ClientOptions{Authentication: true},
						},
					},
				},
			},
			enterpriseEnabled: true,
			expectErrors:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := license.MockLicenseChecker{EnterpriseEnabled: tt.enterpriseEnabled}
			actual := validClientAuthentication(context.Background(), tt.es, checker)
			actualErrors := len(actual) > 0
			if tt.expectErrors != actualErrors {
				t.Errorf("failed validClientAuthentication(). Name: %v, actual %v, wanted errors: %v", tt.name, actual, tt.expectErrors)
			}
		})
	}
}

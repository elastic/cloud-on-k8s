// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package label

import (
	"reflect"
	"testing"

	"github.com/go-test/deep"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"

	v1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
)

func TestClusterFromResourceLabels(t *testing.T) {
	// test when label is not set
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "name",
			Namespace: "namespace",
		},
	}
	accessor, err := meta.Accessor(&pod)
	require.NoError(t, err)
	_, exists := ClusterFromResourceLabels(accessor)
	require.False(t, exists)

	// test when label is set
	pod.ObjectMeta.Labels = map[string]string{ClusterNameLabelName: "clusterName"}
	cluster, exists := ClusterFromResourceLabels(accessor)
	require.True(t, exists)
	require.Equal(t, types.NamespacedName{
		Namespace: "namespace",
		Name:      "clusterName",
	}, cluster)
}

func TestExtractVersion(t *testing.T) {
	tests := []struct {
		name    string
		args    map[string]string
		want    version.Version
		wantErr bool
	}{
		{
			name:    "no version",
			args:    nil,
			wantErr: true,
		},
		{
			name: "invalid version",
			args: map[string]string{
				VersionLabelName: "not a version",
			},
			wantErr: true,
		},
		{
			name: "valid version",
			args: map[string]string{
				VersionLabelName: "1.0.0",
			},
			want:    version.MustParse("1.0.0"),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractVersion(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExtractVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewPodLabels(t *testing.T) {
	type args struct {
		es        types.NamespacedName
		ssetName  string
		ver       version.Version
		nodeRoles *v1.Node
		scheme    string
	}
	nameFixture := types.NamespacedName{
		Namespace: "ns",
		Name:      "name",
	}
	tests := []struct {
		name    string
		args    args
		want    map[string]string
		wantErr bool
	}{
		{
			name: "labels pre-7.3",
			args: args{
				es:       nameFixture,
				ssetName: "sset",
				ver:      version.From(7, 1, 0),
				nodeRoles: &v1.Node{
					Master:    pointer.BoolPtr(false),
					Data:      pointer.BoolPtr(false),
					Ingest:    pointer.BoolPtr(false),
					ML:        pointer.BoolPtr(false),
					Transform: pointer.BoolPtr(false),
				},
				scheme: "https",
			},
			want: map[string]string{
				ClusterNameLabelName:             "name",
				common.TypeLabelName:             "elasticsearch",
				VersionLabelName:                 "7.1.0",
				string(NodeTypesMasterLabelName): "false",
				string(NodeTypesDataLabelName):   "false",
				string(NodeTypesIngestLabelName): "false",
				string(NodeTypesMLLabelName):     "false",
				HTTPSchemeLabelName:              "https",
				StatefulSetNameLabelName:         "sset",
			},
			wantErr: false,
		},
		{
			name: "labels post-7.3",
			args: args{
				es:       nameFixture,
				ssetName: "sset",
				ver:      version.From(7, 3, 0),
				nodeRoles: &v1.Node{
					Master:     pointer.BoolPtr(false),
					Data:       pointer.BoolPtr(true),
					Ingest:     pointer.BoolPtr(false),
					ML:         pointer.BoolPtr(false),
					Transform:  pointer.BoolPtr(true),
					VotingOnly: pointer.BoolPtr(true),
				},
				scheme: "https",
			},
			want: map[string]string{
				ClusterNameLabelName:                 "name",
				common.TypeLabelName:                 "elasticsearch",
				VersionLabelName:                     "7.3.0",
				string(NodeTypesMasterLabelName):     "false",
				string(NodeTypesDataLabelName):       "true",
				string(NodeTypesIngestLabelName):     "false",
				string(NodeTypesMLLabelName):         "false",
				string(NodeTypesVotingOnlyLabelName): "true",
				HTTPSchemeLabelName:                  "https",
				StatefulSetNameLabelName:             "sset",
			},
			wantErr: false,
		},
		{
			name: "labels post-7.7",
			args: args{
				es:       nameFixture,
				ssetName: "sset",
				ver:      version.From(7, 7, 0),
				nodeRoles: &v1.Node{
					Master:    pointer.BoolPtr(false),
					Data:      pointer.BoolPtr(true),
					Ingest:    pointer.BoolPtr(false),
					ML:        pointer.BoolPtr(false),
					Transform: pointer.BoolPtr(true),
				},
				scheme: "https",
			},
			want: map[string]string{
				ClusterNameLabelName:                          "name",
				common.TypeLabelName:                          "elasticsearch",
				VersionLabelName:                              "7.7.0",
				string(NodeTypesMasterLabelName):              "false",
				string(NodeTypesDataLabelName):                "true",
				string(NodeTypesIngestLabelName):              "false",
				string(NodeTypesMLLabelName):                  "false",
				string(NodeTypesTransformLabelName):           "true",
				string(NodeTypesRemoteClusterClientLabelName): "true",
				string(NodeTypesVotingOnlyLabelName):          "false",
				HTTPSchemeLabelName:                           "https",
				StatefulSetNameLabelName:                      "sset",
			},
			wantErr: false,
		},
		{
			name: "labels post-7.10",
			args: args{
				es:       nameFixture,
				ssetName: "sset",
				ver:      version.From(7, 10, 0),
				nodeRoles: &v1.Node{
					Roles: nil,
				},
				scheme: "https",
			},
			want: map[string]string{
				ClusterNameLabelName:                          "name",
				common.TypeLabelName:                          "elasticsearch",
				VersionLabelName:                              "7.10.0",
				string(NodeTypesMasterLabelName):              "true",
				string(NodeTypesDataLabelName):                "true",
				string(NodeTypesDataHotLabelName):             "true",
				string(NodeTypesDataWarmLabelName):            "true",
				string(NodeTypesDataContentLabelName):         "true",
				string(NodeTypesDataColdLabelName):            "true",
				string(NodeTypesIngestLabelName):              "true",
				string(NodeTypesMLLabelName):                  "true",
				string(NodeTypesTransformLabelName):           "true",
				string(NodeTypesRemoteClusterClientLabelName): "true",
				string(NodeTypesVotingOnlyLabelName):          "false",
				HTTPSchemeLabelName:                           "https",
				StatefulSetNameLabelName:                      "sset",
			},
			wantErr: false,
		},
		{
			name: "labels post-7.12",
			args: args{
				es:       nameFixture,
				ssetName: "sset",
				ver:      version.From(7, 12, 0),
				nodeRoles: &v1.Node{
					Roles: nil,
				},
				scheme: "https",
			},
			want: map[string]string{
				ClusterNameLabelName:                          "name",
				common.TypeLabelName:                          "elasticsearch",
				VersionLabelName:                              "7.12.0",
				string(NodeTypesMasterLabelName):              "true",
				string(NodeTypesDataLabelName):                "true",
				string(NodeTypesDataHotLabelName):             "true",
				string(NodeTypesDataWarmLabelName):            "true",
				string(NodeTypesDataContentLabelName):         "true",
				string(NodeTypesDataColdLabelName):            "true",
				string(NodeTypesDataFrozenLabelName):          "true",
				string(NodeTypesIngestLabelName):              "true",
				string(NodeTypesMLLabelName):                  "true",
				string(NodeTypesTransformLabelName):           "true",
				string(NodeTypesRemoteClusterClientLabelName): "true",
				string(NodeTypesVotingOnlyLabelName):          "false",
				HTTPSchemeLabelName:                           "https",
				StatefulSetNameLabelName:                      "sset",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewPodLabels(tt.args.es, tt.args.ssetName, tt.args.ver, tt.args.nodeRoles, tt.args.scheme)
			require.Nil(t, deep.Equal(got, tt.want))
		})
	}
}

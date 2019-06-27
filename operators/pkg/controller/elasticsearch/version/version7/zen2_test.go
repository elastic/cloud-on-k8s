// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version7

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/mutation"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pod"
)

type ExpectedCall struct {
	clearVotingExclusions     bool
	addVotingConfigExclusions bool
	mastersToAdd              []string
}

type FakeESClient struct {
	esclient.Client
	calls []ExpectedCall
}

func (f *FakeESClient) AddVotingConfigExclusions(ctx context.Context, nodeNames []string, timeout string) error {
	f.calls = append(f.calls, ExpectedCall{
		addVotingConfigExclusions: true,
		mastersToAdd:              nodeNames,
	})
	return nil
}

func (f *FakeESClient) DeleteVotingConfigExclusions(ctx context.Context, waitForRemoval bool) error {
	f.calls = append(f.calls, ExpectedCall{
		clearVotingExclusions: true,
	})
	return nil
}

func TestUpdateZen2Settings(t *testing.T) {
	type args struct {
		esClient           esclient.Client
		minVersion         version.Version
		performableChanges mutation.PerformableChanges
		podsState          mutation.PodsState
	}
	master1 := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "master1",
			Labels: label.NodeTypesMasterLabelName.AsMap(true),
		},
	}
	master2 := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "master2",
			Labels: label.NodeTypesMasterLabelName.AsMap(true),
		},
	}
	data1 := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "data1",
			Labels: label.NodeTypesDataLabelName.AsMap(true),
		},
	}
	tests := []struct {
		name string
		args args
		want []ExpectedCall
	}{
		{
			name: "Mixed clusters with pre-7.x.x nodes, don't use zen2 API",
			args: args{
				esClient:   &FakeESClient{},
				minVersion: version.MustParse("6.8.0"),
				performableChanges: mutation.PerformableChanges{
					Changes: mutation.Changes{
						ToCreate: nil,
						ToKeep:   nil,
						ToDelete: mutation.PodsToDelete{
							{PodWithConfig: pod.PodWithConfig{Pod: master1}},
						},
					},
				},
			},
			want: nil,
		},
		{
			name: "No changes: delete voting exclusions",
			args: args{
				esClient:           &FakeESClient{},
				minVersion:         version.MustParse("7.0.0"),
				performableChanges: mutation.PerformableChanges{},
			},
			want: []ExpectedCall{{clearVotingExclusions: true}},
		},
		{
			name: "Master deletion in progress, another master to delete: add both, don't clear",
			args: args{
				esClient:   &FakeESClient{},
				minVersion: version.MustParse("7.0.0"),
				performableChanges: mutation.PerformableChanges{
					Changes: mutation.Changes{
						ToCreate: nil,
						ToKeep:   nil,
						ToDelete: mutation.PodsToDelete{
							{PodWithConfig: pod.PodWithConfig{Pod: master1}},
							{PodWithConfig: pod.PodWithConfig{Pod: master2}},
						},
					},
				},
				podsState: mutation.PodsState{
					Deleting: map[string]corev1.Pod{
						master1.Name: master1,
					},
				},
			},
			want: []ExpectedCall{
				{addVotingConfigExclusions: true, mastersToAdd: []string{master1.Name, master2.Name}},
			},
		},
		{
			name: "Master to delete: clear voting exclusions then delete the master",
			args: args{
				esClient:   &FakeESClient{},
				minVersion: version.MustParse("7.0.0"),
				performableChanges: mutation.PerformableChanges{
					Changes: mutation.Changes{
						ToCreate: nil,
						ToKeep:   nil,
						ToDelete: mutation.PodsToDelete{
							{PodWithConfig: pod.PodWithConfig{Pod: master1}},
						},
					},
				},
			},
			want: []ExpectedCall{
				{clearVotingExclusions: true},
				{addVotingConfigExclusions: true, mastersToAdd: []string{master1.Name}},
			},
		},
		{
			name: "Master and data delete: only set voting exclusions for master nodes",
			args: args{
				esClient:   &FakeESClient{},
				minVersion: version.MustParse("7.0.0"),
				performableChanges: mutation.PerformableChanges{
					Changes: mutation.Changes{
						ToCreate: nil,
						ToKeep:   nil,
						ToDelete: mutation.PodsToDelete{
							{PodWithConfig: pod.PodWithConfig{Pod: master1}},
							{PodWithConfig: pod.PodWithConfig{Pod: master2}},
							{PodWithConfig: pod.PodWithConfig{Pod: data1}},
						},
					},
				},
			},
			want: []ExpectedCall{
				{clearVotingExclusions: true},
				{addVotingConfigExclusions: true, mastersToAdd: []string{master1.Name, master2.Name}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := UpdateZen2Settings(tt.args.esClient, tt.args.minVersion, tt.args.performableChanges, tt.args.podsState)
			require.NoError(t, err)
			require.Equal(t, tt.want, tt.args.esClient.(*FakeESClient).calls)
		})
	}
}

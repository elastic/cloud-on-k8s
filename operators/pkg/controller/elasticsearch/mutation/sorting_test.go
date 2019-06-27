// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package mutation

import (
	"sort"
	"testing"
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pod"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func Test_sortPodsByMasterNodeLastAndCreationTimestampAsc(t *testing.T) {
	masterNode := namedPodWithCreationTimestamp("master", time.Unix(5, 0))

	type args struct {
		terminal   map[string]corev1.Pod
		masterNode *pod.PodWithConfig
		pods       PodsToDelete
	}
	tests := []struct {
		name string
		args args
		want PodsToDelete
	}{
		{
			name: "sample",
			args: args{
				masterNode: &masterNode,
				pods: podsToDelete(
					masterNode,
					namedPodWithCreationTimestamp("4", time.Unix(4, 0)),
					namedPodWithCreationTimestamp("3", time.Unix(3, 0)),
					namedPodWithCreationTimestamp("6", time.Unix(6, 0)),
				),
			},
			want: podsToDelete(
				namedPodWithCreationTimestamp("3", time.Unix(3, 0)),
				namedPodWithCreationTimestamp("4", time.Unix(4, 0)),
				namedPodWithCreationTimestamp("6", time.Unix(6, 0)),
				masterNode,
			),
		},
		{
			name: "terminal pods first",
			args: args{
				masterNode: &masterNode,
				pods: podsToDelete(
					masterNode,
					namedPodWithCreationTimestamp("4", time.Unix(4, 0)),
					namedPodWithCreationTimestamp("3", time.Unix(3, 0)),
					namedPodWithCreationTimestamp("6", time.Unix(6, 0)),
				),
				terminal: map[string]corev1.Pod{"6": namedPod("6").Pod},
			},
			want: podsToDelete(
				namedPodWithCreationTimestamp("6", time.Unix(6, 0)),
				namedPodWithCreationTimestamp("3", time.Unix(3, 0)),
				namedPodWithCreationTimestamp("4", time.Unix(4, 0)),
				masterNode,
			),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sort.SliceStable(
				tt.args.pods,
				sortPodsByTerminalFirstMasterNodeLastAndCreationTimestampAsc(
					tt.args.terminal,
					&tt.args.masterNode.Pod,
					tt.args.pods,
				),
			)

			assert.Equal(t, tt.want, tt.args.pods)
		})
	}
}

func Test_sortPodsToCreateByMasterNodesFirstThenNameAsc(t *testing.T) {
	masterNode5 := PodToCreate{Pod: namedPodWithCreationTimestamp("master5", time.Unix(5, 0)).Pod}
	masterNode5.Pod.Labels = label.NodeTypesMasterLabelName.AsMap(true)
	masterNode6 := PodToCreate{Pod: namedPodWithCreationTimestamp("master6", time.Unix(6, 0)).Pod}
	masterNode6.Pod.Labels = label.NodeTypesMasterLabelName.AsMap(true)

	type args struct {
		pods []PodToCreate
	}
	tests := []struct {
		name string
		args args
		want []PodToCreate
	}{
		{
			name: "sample",
			args: args{
				pods: []PodToCreate{
					{Pod: namedPodWithCreationTimestamp("4", time.Unix(4, 0)).Pod},
					masterNode6,
					{Pod: namedPodWithCreationTimestamp("3", time.Unix(3, 0)).Pod},
					masterNode5,
					{Pod: namedPodWithCreationTimestamp("6", time.Unix(6, 0)).Pod},
				},
			},
			want: []PodToCreate{
				masterNode5,
				masterNode6,
				{Pod: namedPodWithCreationTimestamp("3", time.Unix(3, 0)).Pod},
				{Pod: namedPodWithCreationTimestamp("4", time.Unix(4, 0)).Pod},
				{Pod: namedPodWithCreationTimestamp("6", time.Unix(6, 0)).Pod},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sort.SliceStable(
				tt.args.pods,
				sortPodsToCreateByMasterNodesFirstThenNameAsc(tt.args.pods),
			)

			assert.Equal(t, tt.want, tt.args.pods)
		})
	}
}

func Test_sortPodtoDeleteByCreationTimestampAsc(t *testing.T) {
	tests := []struct {
		name string
		pods PodsToDelete
		want PodsToDelete
	}{
		{
			name: "sample",
			pods: PodsToDelete{
				{PodWithConfig: namedPodWithCreationTimestamp("4", time.Unix(4, 0))},
				{PodWithConfig: namedPodWithCreationTimestamp("3", time.Unix(3, 0))},
				{PodWithConfig: namedPodWithCreationTimestamp("6", time.Unix(6, 0))},
			},
			want: PodsToDelete{
				{PodWithConfig: namedPodWithCreationTimestamp("3", time.Unix(3, 0))},
				{PodWithConfig: namedPodWithCreationTimestamp("4", time.Unix(4, 0))},
				{PodWithConfig: namedPodWithCreationTimestamp("6", time.Unix(6, 0))},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sort.SliceStable(tt.pods, sortPodtoDeleteByCreationTimestampAsc(tt.pods))
			assert.Equal(t, tt.want, tt.pods)
		})
	}
}

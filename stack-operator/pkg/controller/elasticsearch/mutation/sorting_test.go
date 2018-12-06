package mutation

import (
	"sort"
	"testing"
	"time"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func Test_sortPodsByMasterNodeLastAndCreationTimestampAsc(t *testing.T) {
	masterNode := namedPodWithCreationTimestamp("master", time.Unix(5, 0))

	type args struct {
		masterNode *corev1.Pod
		pods       []corev1.Pod
	}
	tests := []struct {
		name string
		args args
		want []corev1.Pod
	}{
		{
			name: "sample",
			args: args{
				masterNode: &masterNode,
				pods: []corev1.Pod{
					masterNode,
					namedPodWithCreationTimestamp("4", time.Unix(4, 0)),
					namedPodWithCreationTimestamp("3", time.Unix(3, 0)),
					namedPodWithCreationTimestamp("6", time.Unix(6, 0)),
				},
			},
			want: []corev1.Pod{
				namedPodWithCreationTimestamp("3", time.Unix(3, 0)),
				namedPodWithCreationTimestamp("4", time.Unix(4, 0)),
				namedPodWithCreationTimestamp("6", time.Unix(6, 0)),
				masterNode,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sort.SliceStable(
				tt.args.pods,
				sortPodsByMasterNodeLastAndCreationTimestampAsc(tt.args.masterNode, tt.args.pods),
			)

			assert.Equal(t, tt.want, tt.args.pods)
		})
	}
}

func Test_sortPodsByMasterNodesFirstThenNameAsc(t *testing.T) {
	masterNode5 := namedPodWithCreationTimestamp("master5", time.Unix(5, 0))
	masterNode5.Labels = support.NodeTypesMasterLabelName.AsMap(true)
	masterNode6 := namedPodWithCreationTimestamp("master6", time.Unix(6, 0))
	masterNode6.Labels = support.NodeTypesMasterLabelName.AsMap(true)

	type args struct {
		pods []corev1.Pod
	}
	tests := []struct {
		name string
		args args
		want []corev1.Pod
	}{
		{
			name: "sample",
			args: args{
				pods: []corev1.Pod{
					namedPodWithCreationTimestamp("4", time.Unix(4, 0)),
					masterNode6,
					namedPodWithCreationTimestamp("3", time.Unix(3, 0)),
					masterNode5,
					namedPodWithCreationTimestamp("6", time.Unix(6, 0)),
				},
			},
			want: []corev1.Pod{
				masterNode5,
				masterNode6,
				namedPodWithCreationTimestamp("3", time.Unix(3, 0)),
				namedPodWithCreationTimestamp("4", time.Unix(4, 0)),
				namedPodWithCreationTimestamp("6", time.Unix(6, 0)),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sort.SliceStable(
				tt.args.pods,
				sortPodsByMasterNodesFirstThenNameAsc(tt.args.pods),
			)

			assert.Equal(t, tt.want, tt.args.pods)
		})
	}
}

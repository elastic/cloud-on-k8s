package mutation

import (
	"reflect"
	"testing"

	"github.com/elastic/k8s-operators/stack-operator/pkg/controller/elasticsearch/label"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func podListToSetLike(pods []corev1.Pod) map[string]struct{} {
	result := make(map[string]struct{})
	for _, pod := range pods {
		result[pod.Name] = empty
	}
	return result
}

func TestNewPodRestrictions(t *testing.T) {
	masterPod := withLabels(namedPod("master"), label.NodeTypesMasterLabelName.AsMap(true))
	dataPod := withLabels(namedPod("data"), label.NodeTypesDataLabelName.AsMap(true))

	type args struct {
		podsState PodsState
	}
	tests := []struct {
		name string
		args args
		want PodRestrictions
	}{
		{
			name: "uses RunningReady state",
			args: args{
				podsState: initializePodsState(PodsState{
					RunningReady: podListToMap([]corev1.Pod{
						namedPod("foo"),
						masterPod,
						dataPod,
					}),
				}),
			},
			want: PodRestrictions{
				MasterNodeNames: podListToSetLike([]corev1.Pod{masterPod}),
				DataNodeNames:   podListToSetLike([]corev1.Pod{dataPod}),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewPodRestrictions(tt.args.podsState); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewPodRestrictions() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPodRestrictions_CanDelete(t *testing.T) {
	masterPod := withLabels(namedPod("master"), label.NodeTypesMasterLabelName.AsMap(true))
	dataPod := withLabels(namedPod("data"), label.NodeTypesDataLabelName.AsMap(true))

	type args struct {
		pod corev1.Pod
	}
	tests := []struct {
		name            string
		podRestrictions PodRestrictions
		args            args
		wantErr         error
	}{
		{
			name: "cant delete last master node",
			podRestrictions: PodRestrictions{
				MasterNodeNames: podListToSetLike([]corev1.Pod{masterPod}),
			},
			args: args{
				pod: masterPod,
			},
			wantErr: ErrNotEnoughMasterEligiblePods,
		},
		{
			name: "can delete non-last master node",
			podRestrictions: PodRestrictions{
				MasterNodeNames: podListToSetLike([]corev1.Pod{masterPod, namedPod("bar")}),
			},
			args: args{
				pod: masterPod,
			},
		},
		{
			name: "cant delete last data node",
			podRestrictions: PodRestrictions{
				DataNodeNames: podListToSetLike([]corev1.Pod{dataPod}),
			},
			args: args{
				pod: dataPod,
			},
			wantErr: ErrNotEnoughDataEligiblePods,
		},
		{
			name: "can delete non-last data node",
			podRestrictions: PodRestrictions{
				DataNodeNames: podListToSetLike([]corev1.Pod{dataPod, namedPod("bar")}),
			},
			args: args{
				pod: dataPod,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.podRestrictions.CanDelete(tt.args.pod)

			if tt.wantErr == nil {
				assert.NoError(t, err)
			} else {
				assert.Equal(t, tt.wantErr, err)
			}
		})
	}
}

func TestPodRestrictions_Remove(t *testing.T) {
	type args struct {
		pod corev1.Pod
	}
	tests := []struct {
		name            string
		podRestrictions PodRestrictions
		args            args
		want            PodRestrictions
	}{
		{
			name: "can delete",
			podRestrictions: PodRestrictions{
				MasterNodeNames: podListToSetLike([]corev1.Pod{namedPod("foo"), namedPod("bar")}),
				DataNodeNames:   podListToSetLike([]corev1.Pod{namedPod("foo"), namedPod("bar")}),
			},
			args: args{
				pod: namedPod("foo"),
			},
			want: PodRestrictions{
				MasterNodeNames: podListToSetLike([]corev1.Pod{namedPod("bar")}),
				DataNodeNames:   podListToSetLike([]corev1.Pod{namedPod("bar")}),
			},
		},
		{
			name: "can delete nonexistent without failing",
			args: args{
				pod: namedPod("foo"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.podRestrictions.Remove(tt.args.pod)

			assert.Equal(t, tt.want, tt.podRestrictions)
		})
	}
}

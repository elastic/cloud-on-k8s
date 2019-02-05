package mutation

import (
	"testing"

	"github.com/elastic/k8s-operators/stack-operator/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/stack-operator/pkg/controller/elasticsearch/observer"
	"github.com/elastic/k8s-operators/stack-operator/pkg/controller/elasticsearch/reconcile"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestNewPodsState(t *testing.T) {
	exampleMasterNodePod := namedPod("master")

	type args struct {
		resourcesState reconcile.ResourcesState
		observedState  observer.State
	}
	tests := []struct {
		name string
		args args
		want PodsState
	}{
		{
			name: "should bucket pods into the expected states",
			args: args{
				resourcesState: reconcile.ResourcesState{
					CurrentPodsByPhase: map[corev1.PodPhase][]corev1.Pod{
						corev1.PodPending:   {namedPod("1")},
						corev1.PodRunning:   {exampleMasterNodePod, namedPod("2"), namedPod("3")},
						corev1.PodUnknown:   {namedPod("5")},
						corev1.PodFailed:    {namedPod("6")},
						corev1.PodSucceeded: {namedPod("7")},
					},
					DeletingPods: []corev1.Pod{namedPod("8")},
				},
				observedState: observer.State{
					ClusterState: &client.ClusterState{
						MasterNode: "master-node-id",
						Nodes: map[string]client.ClusterStateNode{
							"master-node-id": {Name: exampleMasterNodePod.Name},
							"a":              {Name: "3"},
						},
					},
				},
			},
			want: PodsState{
				Pending:        map[string]corev1.Pod{"1": namedPod("1")},
				RunningJoining: map[string]corev1.Pod{"2": namedPod("2")},
				RunningReady:   map[string]corev1.Pod{"master": exampleMasterNodePod, "3": namedPod("3")},
				RunningUnknown: map[string]corev1.Pod{},
				Unknown:        map[string]corev1.Pod{"5": namedPod("5")},
				Terminal:       map[string]corev1.Pod{"6": namedPod("6"), "7": namedPod("7")},
				Deleting:       map[string]corev1.Pod{"8": namedPod("8")},

				MasterNodePod: &exampleMasterNodePod,
			},
		},
		{
			name: "should bucket pods into the expected states when no cluster state is available",
			args: args{
				resourcesState: reconcile.ResourcesState{
					CurrentPodsByPhase: map[corev1.PodPhase][]corev1.Pod{
						corev1.PodPending:   {namedPod("1")},
						corev1.PodRunning:   {exampleMasterNodePod, namedPod("2"), namedPod("3")},
						corev1.PodUnknown:   {namedPod("5")},
						corev1.PodFailed:    {namedPod("6")},
						corev1.PodSucceeded: {namedPod("7")},
					},
					DeletingPods: []corev1.Pod{namedPod("8")},
				},
				observedState: observer.State{},
			},
			want: PodsState{
				Pending:        map[string]corev1.Pod{"1": namedPod("1")},
				RunningJoining: map[string]corev1.Pod{},
				RunningReady:   map[string]corev1.Pod{},
				RunningUnknown: map[string]corev1.Pod{
					"2":      namedPod("2"),
					"master": exampleMasterNodePod,
					"3":      namedPod("3"),
				},
				Unknown:  map[string]corev1.Pod{"5": namedPod("5")},
				Terminal: map[string]corev1.Pod{"6": namedPod("6"), "7": namedPod("7")},
				Deleting: map[string]corev1.Pod{"8": namedPod("8")},

				MasterNodePod: nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewPodsState(tt.args.resourcesState, tt.args.observedState)

			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_NewEmptyPodsState(t *testing.T) {
	s := NewEmptyPodsState()

	assert.Nil(t, s.MasterNodePod)

	assert.NotNil(t, s.Pending)
	assert.NotNil(t, s.RunningJoining)
	assert.NotNil(t, s.RunningReady)
	assert.NotNil(t, s.RunningUnknown)
	assert.NotNil(t, s.Unknown)
	assert.NotNil(t, s.Terminal)
	assert.NotNil(t, s.Deleting)
}

func TestPodsState_CurrentPodsCount(t *testing.T) {
	tests := []struct {
		name      string
		podsState PodsState
		want      int
	}{
		{
			name: "should count all non-terminal pods",
			podsState: PodsState{
				Pending:        map[string]corev1.Pod{"1": {}},
				RunningJoining: map[string]corev1.Pod{"2": {}},
				RunningReady:   map[string]corev1.Pod{"3": {}},
				RunningUnknown: map[string]corev1.Pod{"4": {}},
				Unknown:        map[string]corev1.Pod{"5": {}},
				Terminal:       map[string]corev1.Pod{"6": {}, "6.1": {}, "6.2": {}, "6.3": {}, "6.4": {}, "6.5": {}},
				Deleting:       map[string]corev1.Pod{"7": {}},
			},
			want: 6,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.podsState
			if got := s.CurrentPodsCount(); got != tt.want {
				t.Errorf("PodsState.CurrentPodsCount() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPodsState_Partition(t *testing.T) {
	type args struct {
		changes Changes
	}
	tests := []struct {
		name      string
		podsState PodsState
		args      args
		want      PodsState
		want1     PodsState
	}{
		{
			name: "a sample set",
			podsState: PodsState{
				Pending:        map[string]corev1.Pod{"1": namedPod("1")},
				RunningJoining: map[string]corev1.Pod{"2": namedPod("2")},
				RunningReady:   map[string]corev1.Pod{"3": namedPod("3")},
				RunningUnknown: map[string]corev1.Pod{"4": namedPod("4")},
				Unknown:        map[string]corev1.Pod{"5": namedPod("5")},
				Terminal:       map[string]corev1.Pod{"6": namedPod("6")},
				Deleting:       map[string]corev1.Pod{"7": namedPod("7")},
			},
			args: args{
				changes: Changes{
					ToDelete: []corev1.Pod{namedPod("2")},
					ToKeep:   []corev1.Pod{namedPod("3")},
					// expecting this to be ignored, and just kept in the remainder.
					ToCreate: []PodToCreate{{Pod: namedPod("4")}},
				},
			},
			want: initializePodsState(PodsState{
				RunningJoining: map[string]corev1.Pod{"2": namedPod("2")},
				RunningReady:   map[string]corev1.Pod{"3": namedPod("3")},
			}),
			want1: initializePodsState(PodsState{
				Pending:        map[string]corev1.Pod{"1": namedPod("1")},
				RunningUnknown: map[string]corev1.Pod{"4": namedPod("4")},
				Unknown:        map[string]corev1.Pod{"5": namedPod("5")},
				Terminal:       map[string]corev1.Pod{"6": namedPod("6")},
				Deleting:       map[string]corev1.Pod{"7": namedPod("7")},
			}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.podsState
			got, got1 := s.Partition(tt.args.changes)

			assert.Equal(t, tt.want, got, "PodsState.Partition() got")
			assert.Equal(t, tt.want1, got1, "PodsState.Partition() got1")
		})
	}
}

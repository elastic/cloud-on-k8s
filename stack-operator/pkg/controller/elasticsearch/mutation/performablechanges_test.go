package mutation

import (
	"fmt"
	"testing"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPerformableChanges_IsEmpty(t *testing.T) {
	tests := []struct {
		name    string
		changes PerformableChanges
		want    bool
	}{
		{name: "empty", changes: PerformableChanges{}, want: true},
		{name: "creation", changes: PerformableChanges{ScheduleForCreation: []CreatablePod{{}}}, want: false},
		{name: "deletion", changes: PerformableChanges{ScheduleForDeletion: []corev1.Pod{{}}}, want: false},
		{
			name: "creation and deletion",
			changes: PerformableChanges{
				ScheduleForCreation: []CreatablePod{{}},
				ScheduleForDeletion: []corev1.Pod{{}},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.changes
			if got := c.IsEmpty(); got != tt.want {
				t.Errorf("PerformableChanges.IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func generatePodsN(n int, namePrefix string, labels map[string]string) []corev1.Pod {
	pods := make([]corev1.Pod, n)
	for i := range pods {
		pods[i] = withLabels(namedPod(fmt.Sprintf("%s%d", namePrefix, i)), labels)
	}
	return pods
}

func podListToMap(pods []corev1.Pod) map[string]corev1.Pod {
	result := make(map[string]corev1.Pod)
	for _, pod := range pods {
		result[pod.Name] = pod
	}
	return result
}

func concatPodList(podLists ...[]corev1.Pod) []corev1.Pod {
	res := make([]corev1.Pod, 0)
	for _, pods := range podLists {
		res = append(res, pods...)
	}
	return res
}

func TestCalculatePerformableChanges(t *testing.T) {
	podsA := generatePodsN(4, "a-", map[string]string{"zone": "a"})
	podsB := generatePodsN(4, "b-", map[string]string{"zone": "b"})
	podsC := generatePodsN(4, "c-", map[string]string{"zone": "c"})

	updateStrategyWithZonesAsGroups := v1alpha1.UpdateStrategy{
		Groups: []v1alpha1.GroupingDefinition{
			{Selector: v1.LabelSelector{MatchLabels: map[string]string{"zone": "a"}}},
			{Selector: v1.LabelSelector{MatchLabels: map[string]string{"zone": "b"}}},
			{Selector: v1.LabelSelector{MatchLabels: map[string]string{"zone": "c"}}},
		},
	}

	type args struct {
		strategy      v1alpha1.UpdateStrategy
		allPodChanges *ChangeSet
		allPodsState  PodsState
	}

	tests := []struct {
		name    string
		args    args
		want    *PerformableChanges
		wantErr bool
	}{
		{
			name: "basic scale-down with a failed zone",
			args: args{
				strategy: v1alpha1.UpdateStrategy{},
				allPodChanges: &ChangeSet{
					ToKeep:   concatPodList(podsA[:2], podsC[:2]),
					ToRemove: concatPodList(podsB[:2]),
				},
				allPodsState: initializePodsState(PodsState{
					RunningReady: podListToMap(concatPodList(podsA[:2], podsC[:2])),
					Terminal:     podListToMap(podsB[:2]),
				}),
			},
			want: &PerformableChanges{
				ScheduleForDeletion: concatPodList(podsB[:2]),
			},
		},
		{
			name: "scale-down with groups",
			args: args{
				strategy: updateStrategyWithZonesAsGroups,
				allPodChanges: &ChangeSet{
					ToKeep:   concatPodList(podsA[:2], podsC[:2]),
					ToRemove: concatPodList(podsB[:2]),
				},
				allPodsState: initializePodsState(PodsState{
					RunningReady: podListToMap(concatPodList(podsA[:2], podsC[:2])),
					Terminal:     podListToMap(podsB[:2]),
				}),
			},
			want: &PerformableChanges{
				ScheduleForDeletion: concatPodList(podsB[:2]),
			},
		},
		{
			name: "multi-zone failure recovery during rolling change without groups",
			args: args{
				strategy: v1alpha1.UpdateStrategy{},
				allPodChanges: &ChangeSet{
					ToAdd:    concatPodList(podsA[2:4], podsB[2:4], podsC[2:4]),
					ToKeep:   concatPodList(),
					ToRemove: concatPodList(podsA[:2], podsB[:2], podsC[:2]),
				},
				allPodsState: initializePodsState(PodsState{
					RunningReady: podListToMap(concatPodList(podsA[:2], podsC[:2])),
					Terminal:     podListToMap(podsB[:2]),
				}),
			},
			want: &PerformableChanges{
				// note that this is not an optimal solution, as zone B is now completely down and we used our change
				// budget trying to rotate nodes in A
				ScheduleForCreation: []CreatablePod{{Pod: podsA[2]}, {Pod: podsA[3]}},
				ScheduleForDeletion: concatPodList(podsB[:2]),

				MaxSurgeGroups:       []string{UnmatchedGroupName, AllGroupName},
				MaxUnavailableGroups: []string{UnmatchedGroupName, AllGroupName},
			},
		},
		{
			name: "multi-zone failure recovery during rolling change with groups",
			args: args{
				strategy: updateStrategyWithZonesAsGroups,
				allPodChanges: &ChangeSet{
					ToAdd:    concatPodList(podsA[2:4], podsB[2:4], podsC[2:4]),
					ToKeep:   concatPodList(),
					ToRemove: concatPodList(podsA[:2], podsB[:2], podsC[:2]),
				},
				allPodsState: initializePodsState(PodsState{
					RunningReady: podListToMap(concatPodList(podsA[:2], podsC[:2])),
					Terminal:     podListToMap(podsB[:2]),
				}),
			},
			want: &PerformableChanges{
				// we might have expected podsA[2] be be created here, but it can't be. why?
				// trivia: which phase does a terminal pod (failed/succeeded) go to when a delete issued?
				ScheduleForCreation: []CreatablePod{{Pod: podsB[2]}, {Pod: podsB[3]}},
				ScheduleForDeletion: concatPodList(podsB[:2]),

				MaxSurgeGroups:       []string{dynamicGroupName(0), dynamicGroupName(2), AllGroupName},
				MaxUnavailableGroups: []string{dynamicGroupName(0), dynamicGroupName(2), AllGroupName},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CalculatePerformableChanges(tt.args.strategy, tt.args.allPodChanges, tt.args.allPodsState)
			if (err != nil) != tt.wantErr {
				t.Errorf("CalculatePerformableChanges() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			assert.Equal(t, tt.want, got)
		})
	}
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package mutation

import (
	"fmt"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pod"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPerformableChanges_HasChanges(t *testing.T) {
	tests := []struct {
		name    string
		changes PerformableChanges
		want    bool
	}{
		{name: "empty", changes: PerformableChanges{}, want: false},
		{
			name:    "creation",
			changes: PerformableChanges{Changes: Changes{ToCreate: []PodToCreate{{}}}},
			want:    true,
		},
		{
			name:    "deletion",
			changes: PerformableChanges{Changes: Changes{ToDelete: pod.PodsWithConfig{{}}}},
			want:    true,
		},
		{
			name: "creation and deletion",
			changes: PerformableChanges{
				Changes: Changes{
					ToCreate: []PodToCreate{{}},
					ToDelete: pod.PodsWithConfig{{}},
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.changes
			if got := c.HasChanges(); got != tt.want {
				t.Errorf("PerformableChanges.HasChanges() = %v, want %v", got, tt.want)
			}
		})
	}
}

func generatePodsN(n int, namePrefix string, labels map[string]string) pod.PodsWithConfig {
	pods := make(pod.PodsWithConfig, n)
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

func concatPodList(podLists ...pod.PodsWithConfig) pod.PodsWithConfig {
	res := make(pod.PodsWithConfig, 0)
	for _, pods := range podLists {
		res = append(res, pods...)
	}
	return res
}

func podToCreateList(pods []corev1.Pod) []PodToCreate {
	res := make([]PodToCreate, 0, len(pods))
	for _, p := range pods {
		res = append(res, PodToCreate{Pod: p})
	}
	return res
}

func TestCalculatePerformableChanges(t *testing.T) {
	podsA := generatePodsN(4, "a-", map[string]string{"zone": "a"})
	podsB := generatePodsN(4, "b-", map[string]string{"zone": "b"})
	podsC := generatePodsN(4, "c-", map[string]string{"zone": "c"})

	updateStrategyWithZonesAsGroups := v1alpha1.UpdateStrategy{
		Groups: []v1alpha1.GroupingDefinition{
			{Selector: metav1.LabelSelector{MatchLabels: map[string]string{"zone": "a"}}},
			{Selector: metav1.LabelSelector{MatchLabels: map[string]string{"zone": "b"}}},
			{Selector: metav1.LabelSelector{MatchLabels: map[string]string{"zone": "c"}}},
		},
	}

	masterDataLabels := label.NodeTypesDataLabelName.AsMap(true)
	label.NodeTypesMasterLabelName.Set(true, masterDataLabels)

	masterDataPods := generatePodsN(2, "master-data-", masterDataLabels)
	masterPods := generatePodsN(2, "master-", label.NodeTypesMasterLabelName.AsMap(true))
	dataPods := generatePodsN(2, "data-", label.NodeTypesDataLabelName.AsMap(true))

	type args struct {
		strategy      v1alpha1.UpdateStrategy
		allPodChanges Changes
		allPodsState  PodsState
	}

	tests := []struct {
		name    string
		args    args
		want    PerformableChanges
		wantErr bool
	}{
		{
			name: "3 dying pods",
			args: args{
				strategy: v1alpha1.UpdateStrategy{},
				allPodChanges: Changes{
					ToCreate: podToCreateList(generatePodsN(3, "new-", map[string]string{"zone": "a"}).Pods()),
				},
				allPodsState: initializePodsState(PodsState{
					Deleting: podListToMap(generatePodsN(3, "old-", map[string]string{"zone": "a"}).Pods()),
				}),
			},
			want: initializePerformableChanges(PerformableChanges{
				Changes: Changes{
					ToCreate: podToCreateList(generatePodsN(1, "new-", map[string]string{"zone": "a"}).Pods()),
				},
				MaxSurgeGroups: []string{UnmatchedGroupName, AllGroupName},
			}),
		},
		{
			name: "scale down two pods",
			args: args{
				strategy: v1alpha1.UpdateStrategy{},
				allPodChanges: Changes{
					ToKeep:   concatPodList(podsA[:2], podsC[:2]),
					ToDelete: concatPodList(podsB[:2]),
				},
				allPodsState: initializePodsState(PodsState{
					RunningReady: podListToMap(concatPodList(podsA[:2], podsB[:2], podsC[:2]).Pods()),
				}),
			},
			want: initializePerformableChanges(PerformableChanges{
				Changes: Changes{
					ToDelete: concatPodList(podsB[:2]),
				},
			}),
		},
		{
			name: "basic scale-down with a failed zone",
			args: args{
				strategy: v1alpha1.UpdateStrategy{},
				allPodChanges: Changes{
					ToKeep:   concatPodList(podsA[:2], podsC[:2]),
					ToDelete: concatPodList(podsB[:2]),
				},
				allPodsState: initializePodsState(PodsState{
					RunningReady: podListToMap(concatPodList(podsA[:2], podsC[:2]).Pods()),
					Terminal:     podListToMap(podsB[:2].Pods()),
				}),
			},
			want: initializePerformableChanges(PerformableChanges{
				Changes: Changes{
					ToDelete: concatPodList(podsB[:2]),
				},
			}),
		},
		{
			name: "scale-down with groups",
			args: args{
				strategy: updateStrategyWithZonesAsGroups,
				allPodChanges: Changes{
					ToKeep:   concatPodList(podsA[:2], podsC[:2]),
					ToDelete: concatPodList(podsB[:2]),
				},
				allPodsState: initializePodsState(PodsState{
					RunningReady: podListToMap(concatPodList(podsA[:2], podsC[:2]).Pods()),
					Terminal:     podListToMap(podsB[:2].Pods()),
				}),
			},
			want: initializePerformableChanges(PerformableChanges{
				Changes: Changes{
					ToDelete: concatPodList(podsB[:2]),
				},
			}),
		},
		{
			name: "multi-zone failure recovery during rolling change without groups",
			args: args{
				strategy: v1alpha1.UpdateStrategy{},
				allPodChanges: Changes{
					ToCreate: podToCreateList(concatPodList(podsA[2:4], podsB[2:4], podsC[2:4]).Pods()),
					ToKeep:   concatPodList(),
					ToDelete: concatPodList(podsA[:2], podsB[:2], podsC[:2]),
				},
				allPodsState: initializePodsState(PodsState{
					RunningReady: podListToMap(concatPodList(podsA[:2], podsC[:2]).Pods()),
					Terminal:     podListToMap(podsB[:2].Pods()),
				}),
			},
			want: initializePerformableChanges(PerformableChanges{
				Changes: Changes{
					// note that this is not an optimal solution, as zone B is now completely down and we used our change
					// budget trying to rotate nodes in A
					// but since no groups where specified, we have no knowledge of a "zone B"
					ToCreate: []PodToCreate{{Pod: podsA[2].Pod}, {Pod: podsA[3].Pod}},
					ToDelete: concatPodList(podsB[:2]),
				},
				MaxSurgeGroups:       []string{UnmatchedGroupName, AllGroupName},
				MaxUnavailableGroups: []string{UnmatchedGroupName, AllGroupName},
			}),
		},
		{
			name: "multi-zone failure recovery during rolling change with groups",
			args: args{
				strategy: updateStrategyWithZonesAsGroups,
				allPodChanges: Changes{
					ToCreate: podToCreateList(concatPodList(podsA[2:4], podsB[2:4], podsC[2:4]).Pods()),
					ToKeep:   concatPodList(),
					ToDelete: concatPodList(podsA[:2], podsB[:2], podsC[:2]),
				},
				allPodsState: initializePodsState(PodsState{
					RunningReady: podListToMap(concatPodList(podsA[:2], podsC[:2]).Pods()),
					Terminal:     podListToMap(podsB[:2].Pods()),
				}),
			},
			want: initializePerformableChanges(PerformableChanges{
				Changes: Changes{
					// we might have expected podsA[2] be be created here, but it can't be. why?
					// trivia: which phase does a terminal pod (failed/succeeded) go to when a delete issued?
					ToCreate: []PodToCreate{{Pod: podsB[2].Pod}, {Pod: podsB[3].Pod}},
					ToDelete: concatPodList(podsB[:2]),
				},

				MaxSurgeGroups:       []string{indexedGroupName(0), indexedGroupName(2), AllGroupName},
				MaxUnavailableGroups: []string{indexedGroupName(0), indexedGroupName(2), AllGroupName},
			}),
		},
		{
			name: "cannot end up without master or data nodes when removing nodes",
			args: args{
				strategy: updateStrategyWithZonesAsGroups,
				allPodChanges: Changes{
					ToKeep:   concatPodList(),
					ToDelete: concatPodList(masterPods, dataPods),
				},
				allPodsState: initializePodsState(PodsState{
					RunningReady: podListToMap(concatPodList(masterPods, dataPods).Pods()),
				}),
			},
			want: initializePerformableChanges(PerformableChanges{
				Changes: Changes{
					ToDelete: concatPodList(masterPods[:1], dataPods[:1]),
				},
				RestrictedPods: map[string]error{
					masterPods[1].Pod.Name: ErrNotEnoughMasterEligiblePods,
					dataPods[1].Pod.Name:   ErrNotEnoughDataEligiblePods,
				},
			}),
		},
		{
			name: "going from mdi node to dedicated m/d nodes",
			args: args{
				strategy: updateStrategyWithZonesAsGroups,
				allPodChanges: Changes{
					ToCreate: podToCreateList(concatPodList(masterPods[:1], dataPods[:1]).Pods()),
					ToKeep:   concatPodList(),
					ToDelete: concatPodList(masterDataPods[:1]),
				},
				allPodsState: initializePodsState(PodsState{
					RunningReady: podListToMap(concatPodList(masterDataPods[:1]).Pods()),
				}),
			},
			want: initializePerformableChanges(PerformableChanges{
				Changes: Changes{
					ToCreate: []PodToCreate{{Pod: masterPods[0].Pod}, {Pod: dataPods[0].Pod}},
				},
				RestrictedPods: map[string]error{
					masterDataPods[0].Pod.Name: ErrNotEnoughMasterEligiblePods,
				},
				MaxSurgeGroups: []string{UnmatchedGroupName},
			}),
		},
		{
			name: "going from dedicated m/d nodes to mdi node",
			args: args{
				strategy: updateStrategyWithZonesAsGroups,
				allPodChanges: Changes{
					ToCreate: podToCreateList(concatPodList(masterDataPods[:1]).Pods()),
					ToKeep:   concatPodList(),
					ToDelete: concatPodList(masterPods[:1], dataPods[:1]),
				},
				allPodsState: initializePodsState(PodsState{
					RunningReady: podListToMap(concatPodList(masterPods[:1], dataPods[:1]).Pods()),
				}),
			},
			want: initializePerformableChanges(PerformableChanges{
				Changes: Changes{
					ToCreate: []PodToCreate{{Pod: masterDataPods[0].Pod}},
				},
				RestrictedPods: map[string]error{
					masterPods[0].Pod.Name: ErrNotEnoughMasterEligiblePods,
					dataPods[0].Pod.Name:   ErrNotEnoughDataEligiblePods,
				},
				MaxSurgeGroups: []string{UnmatchedGroupName, AllGroupName},
			}),
		},
		{
			name: "going from dedicated m/d nodes to mdi node with an existing mdi node",
			args: args{
				strategy: updateStrategyWithZonesAsGroups,
				allPodChanges: Changes{
					ToCreate: podToCreateList(concatPodList(masterDataPods[:1]).Pods()),
					ToKeep:   concatPodList(masterDataPods[1:]),
					ToDelete: concatPodList(masterPods[:1], dataPods[:1]),
				},
				allPodsState: initializePodsState(PodsState{
					RunningJoining: podListToMap(concatPodList(masterDataPods[1:]).Pods()),
					RunningReady:   podListToMap(concatPodList(masterPods[:1], dataPods[:1]).Pods()),
				}),
			},
			want: initializePerformableChanges(PerformableChanges{
				Changes: Changes{},
				// we have to wait for the mdi node to join before we can start deleting master/data nodes
				RestrictedPods: map[string]error{
					masterPods[0].Pod.Name: ErrNotEnoughMasterEligiblePods,
					dataPods[0].Pod.Name:   ErrNotEnoughDataEligiblePods,
				},
				MaxSurgeGroups: []string{UnmatchedGroupName, AllGroupName},
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CalculatePerformableChanges(tt.args.strategy, tt.args.allPodChanges, tt.args.allPodsState)
			if (err != nil) != tt.wantErr {
				t.Errorf("CalculatePerformableChanges() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			assert.Equal(t, tt.want, *got)
		})
	}
}

package version7

import (
	"testing"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/label"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/mutation"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/reconcilehelper"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/settings"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// newPod creates a new named potentially labeled as master
func newPod(name string, master bool) corev1.Pod {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: make(map[string]string),
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{}},
		},
	}

	label.NodeTypesMasterLabelName.Set(master, pod.Labels)

	return pod
}

// buildInitialMasterNodesByPod conveniently summarizes the initial_master_nodes setting by the pod names
func buildInitialMasterNodesByPod(changes *mutation.PerformableChanges) map[string]string {
	res := make(map[string]string)

pod:
	for _, change := range changes.ToCreate {
		for _, container := range change.Pod.Spec.Containers {
			for _, env := range container.Env {
				if env.Name == settings.EnvClusterInitialMasterNodes {
					res[change.Pod.Name] = env.Value
					continue pod
				}
			}
		}
	}

	return res
}

func TestClusterInitialMasterNodesEnforcer(t *testing.T) {
	type args struct {
		performableChanges mutation.PerformableChanges
		resourcesState     reconcilehelper.ResourcesState
	}
	tests := []struct {
		name       string
		args       args
		assertions func(t *testing.T, changes *mutation.PerformableChanges)
		wantErr    bool
	}{
		{
			name: "not set when likely already bootstrapped",
			args: args{
				performableChanges: mutation.PerformableChanges{
					Changes: mutation.Changes{
						ToCreate: []mutation.PodToCreate{{
							Pod: newPod("b", true),
						}},
					},
				},
				resourcesState: reconcilehelper.ResourcesState{
					CurrentPods: []corev1.Pod{newPod("a", true)},
				},
			},
			assertions: func(t *testing.T, changes *mutation.PerformableChanges) {
				initialMasterNodesByPod := buildInitialMasterNodesByPod(changes)
				assert.Empty(t, initialMasterNodesByPod)
			},
		},
		{
			name: "set when likely not bootstrapped",
			args: args{
				performableChanges: mutation.PerformableChanges{
					Changes: mutation.Changes{
						ToCreate: []mutation.PodToCreate{{
							Pod: newPod("b", true),
						}},
					},
				},
				resourcesState: reconcilehelper.ResourcesState{
					CurrentPods: []corev1.Pod{newPod("a", false)},
				},
			},
			assertions: func(t *testing.T, changes *mutation.PerformableChanges) {
				initialMasterNodesByPod := buildInitialMasterNodesByPod(changes)
				assert.Equal(t, map[string]string{
					"b": "b",
				}, initialMasterNodesByPod)
			},
		},
		{
			name: "all masters are informed of all masters",
			args: args{
				performableChanges: mutation.PerformableChanges{
					Changes: mutation.Changes{
						ToCreate: []mutation.PodToCreate{
							{Pod: newPod("b", true)},
							{Pod: newPod("c", true)},
							{Pod: newPod("d", true)},
							{Pod: newPod("e", true)},
							// f is not master, so masters should not be informed of it
							{Pod: newPod("f", false)},
						},
					},
				},
			},
			assertions: func(t *testing.T, changes *mutation.PerformableChanges) {
				initialMasterNodesByPod := buildInitialMasterNodesByPod(changes)
				assert.Equal(t, map[string]string{
					"b": "b,c,d,e",
					"c": "b,c,d,e",
					"d": "b,c,d,e",
					"e": "b,c,d,e",
				}, initialMasterNodesByPod)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ClusterInitialMasterNodesEnforcer(tt.args.performableChanges, tt.args.resourcesState)
			if (err != nil) != tt.wantErr {
				t.Errorf("ClusterInitialMasterNodesEnforcer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			tt.assertions(t, got)
		})
	}
}

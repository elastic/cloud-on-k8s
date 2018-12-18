package reconcilehelper

import (
	"reflect"
	"testing"
	"time"

	v1alpha12 "github.com/elastic/stack-operators/stack-operator/pkg/apis/common/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/events"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestNodesAvailable(t *testing.T) {
	tests := []struct {
		input    []corev1.Pod
		expected int
	}{
		{
			input: []corev1.Pod{
				{
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.PodReady,
								Status: corev1.ConditionTrue,
							},
							{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				{
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.PodReady,
								Status: corev1.ConditionTrue,
							},
							{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionFalse,
							},
						},
					},
				},
			},
			expected: 1,
		},
		{
			input: []corev1.Pod{
				{
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.PodScheduled,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				{
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.PodScheduled,
								Status: corev1.ConditionTrue,
							},
							{
								Type:   corev1.PodReady,
								Status: corev1.ConditionFalse,
							},
							{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionFalse,
							},
						},
					},
				},
			},
			expected: 0,
		},
		{
			input: []corev1.Pod{
				{
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.PodReady,
								Status: corev1.ConditionTrue,
							},
							{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				{
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.PodReady,
								Status: corev1.ConditionTrue,
							},
							{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, len(AvailableElasticsearchNodes(tt.input)))
	}
}

func TestReconcileState_Apply(t *testing.T) {
	tests := []struct {
		name       string
		cluster    v1alpha1.ElasticsearchCluster
		effects    func(s *ReconcileState)
		wantEvents []Event
		wantStatus *v1alpha1.ElasticsearchStatus
	}{
		{
			name:       "defaults",
			cluster:    v1alpha1.ElasticsearchCluster{},
			wantEvents: nil,
			wantStatus: nil,
		},
		{
			name:    "health degraded",
			cluster: v1alpha1.ElasticsearchCluster{},
			effects: func(s *ReconcileState) {
				s.UpdateElasticsearchPending([]corev1.Pod{})
			},
			wantEvents: []Event{{corev1.EventTypeWarning, events.EventReasonUnhealthy, "Elasticsearch cluster health degraded"}},
			wantStatus: &v1alpha1.ElasticsearchStatus{
				ReconcilerStatus: v1alpha12.ReconcilerStatus{
					AvailableNodes: 0,
				},
				Health: v1alpha1.ElasticsearchRedHealth,
				Phase:  v1alpha1.ElasticsearchPendingPhase,
			},
		},
		{
			name: "cluster state lost",
			cluster: v1alpha1.ElasticsearchCluster{
				Status: v1alpha1.ElasticsearchStatus{
					Health:      v1alpha1.ElasticsearchRedHealth,
					ClusterUUID: "old",
				},
			},
			effects: func(s *ReconcileState) {
				s.UpdateElasticsearchOperational(support.ResourcesState{}, support.ObservedState{
					ClusterHealth: &client.Health{
						Status: "red",
					},
					ClusterState: &client.ClusterState{
						ClusterUUID: "new",
					},
				})
			},
			wantEvents: []Event{{corev1.EventTypeWarning, events.EventReasonUnexpected, "Cluster UUID changed (was: old, is: new)"}},
			wantStatus: &v1alpha1.ElasticsearchStatus{
				ReconcilerStatus: v1alpha12.ReconcilerStatus{
					AvailableNodes: 0,
				},
				Health:      v1alpha1.ElasticsearchRedHealth,
				Phase:       v1alpha1.ElasticsearchOperationalPhase,
				ClusterUUID: "new",
			},
		},
		{
			name: "Ignore temporary cluster downtime",
			cluster: v1alpha1.ElasticsearchCluster{
				Status: v1alpha1.ElasticsearchStatus{
					Health:      v1alpha1.ElasticsearchRedHealth,
					ClusterUUID: "old",
				},
			},
			effects: func(s *ReconcileState) {
				s.UpdateElasticsearchOperational(support.ResourcesState{}, support.ObservedState{
					ClusterHealth: &client.Health{
						Status: "red",
					},
					ClusterState: &client.ClusterState{
						ClusterUUID: "",
					},
				})
			},
			wantEvents: nil,
			wantStatus: &v1alpha1.ElasticsearchStatus{
				ReconcilerStatus: v1alpha12.ReconcilerStatus{
					AvailableNodes: 0,
				},
				Health:      v1alpha1.ElasticsearchRedHealth,
				Phase:       v1alpha1.ElasticsearchOperationalPhase,
				ClusterUUID: "old",
			},
		},
		{
			name: "master node changed",
			cluster: v1alpha1.ElasticsearchCluster{
				Status: v1alpha1.ElasticsearchStatus{
					Health:     v1alpha1.ElasticsearchRedHealth,
					MasterNode: "old",
					Phase:      v1alpha1.ElasticsearchOperationalPhase,
				},
			},
			effects: func(s *ReconcileState) {
				s.UpdateElasticsearchState(support.ResourcesState{}, support.ObservedState{
					ClusterHealth: &client.Health{
						Status: "red",
					},
					ClusterState: &client.ClusterState{
						MasterNode: "new",
						Nodes: map[string]client.Node{
							"new": {Name: "new"},
						},
					},
				})
			},
			wantEvents: []Event{{corev1.EventTypeNormal, events.EventReasonStateChange, "Master node is now new"}},
			wantStatus: &v1alpha1.ElasticsearchStatus{
				ReconcilerStatus: v1alpha12.ReconcilerStatus{
					AvailableNodes: 0,
				},
				Health:     v1alpha1.ElasticsearchRedHealth,
				Phase:      v1alpha1.ElasticsearchOperationalPhase,
				MasterNode: "new",
			},
		},
		{
			name: "ignore temporary master loss for status",
			cluster: v1alpha1.ElasticsearchCluster{
				Status: v1alpha1.ElasticsearchStatus{
					Health:     v1alpha1.ElasticsearchRedHealth,
					MasterNode: "old",
					Phase:      v1alpha1.ElasticsearchOperationalPhase,
				},
			},
			effects: func(s *ReconcileState) {
				s.UpdateElasticsearchState(support.ResourcesState{}, support.ObservedState{
					ClusterHealth: &client.Health{
						Status: "red",
					},
					ClusterState: &client.ClusterState{
						MasterNode: "",
					},
				})
			},
			wantEvents: nil,
			wantStatus: &v1alpha1.ElasticsearchStatus{
				ReconcilerStatus: v1alpha12.ReconcilerStatus{
					AvailableNodes: 0,
				},
				Health:     v1alpha1.ElasticsearchRedHealth,
				Phase:      v1alpha1.ElasticsearchOperationalPhase,
				MasterNode: "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewReconcileState(tt.cluster)
			if tt.effects != nil {
				tt.effects(s)
			}
			events, cluster := s.Apply()
			if !reflect.DeepEqual(events, tt.wantEvents) {
				t.Errorf("ReconcileState.Apply() events = %v, wantEvents %v", events, tt.wantEvents)

			}
			var actual *v1alpha1.ElasticsearchStatus
			if cluster != nil {
				actual = &cluster.Status
			}
			if !reflect.DeepEqual(actual, tt.wantStatus) {
				t.Errorf("ReconcileState.Apply() cluster = %v, wantStatus %v", cluster.Status, tt.wantStatus)
			}
		})
	}
}

func TestReconcileState_UpdateElasticsearchState(t *testing.T) {
	type args struct {
		resourcesState support.ResourcesState
		observedState  support.ObservedState
	}
	tests := []struct {
		name            string
		cluster         v1alpha1.ElasticsearchCluster
		args            args
		stateAssertions func(s *ReconcileState)
	}{
		{
			name: "phase is not changed by default",
			cluster: v1alpha1.ElasticsearchCluster{
				Status: v1alpha1.ElasticsearchStatus{
					Phase: v1alpha1.ElasticsearchPendingPhase,
				},
			},
			stateAssertions: func(s *ReconcileState) {
				assert.EqualValues(t, v1alpha1.ElasticsearchPendingPhase, s.status.Phase)
			},
		},
		{
			name:    "health is unknown by default",
			cluster: v1alpha1.ElasticsearchCluster{},
			stateAssertions: func(s *ReconcileState) {
				assert.EqualValues(t, "unknown", s.status.Health)
			},
		},
		{
			name:    "health is set if returned by Elasticsearch",
			cluster: v1alpha1.ElasticsearchCluster{},
			args: args{
				observedState: support.ObservedState{
					ClusterHealth: &client.Health{Status: "green"},
				},
			},
			stateAssertions: func(s *ReconcileState) {
				assert.EqualValues(t, "green", s.status.Health)

			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewReconcileState(tt.cluster)
			s.UpdateElasticsearchState(tt.args.resourcesState, tt.args.observedState)
			if tt.stateAssertions != nil {
				tt.stateAssertions(s)
			}
		})
	}
}

func TestReconcileState_UpdateElasticsearchMigrating(t *testing.T) {
	type args struct {
		result         reconcile.Result
		resourcesState support.ResourcesState
		observedState  support.ObservedState
	}
	tests := []struct {
		name            string
		cluster         v1alpha1.ElasticsearchCluster
		args            args
		stateAssertions func(s *ReconcileState)
	}{
		{
			name:    "base case",
			cluster: v1alpha1.ElasticsearchCluster{},
			args: args{
				result: reconcile.Result{RequeueAfter: 10 * time.Minute},
			},
			stateAssertions: func(s *ReconcileState) {
				assert.EqualValues(t, v1alpha1.ElasticsearchMigratingDataPhase, s.status.Phase)
				assert.Equal(t, []Event{{corev1.EventTypeNormal, events.EventReasonDelayed, "Requested topology change delayed by data migration"}}, s.events)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewReconcileState(tt.cluster)
			s.UpdateElasticsearchMigrating(tt.args.resourcesState, tt.args.observedState)
			if tt.stateAssertions != nil {
				tt.stateAssertions(s)
			}
		})
	}
}

func Test_nextTakesPrecedence(t *testing.T) {
	type args struct {
		current reconcile.Result
		next    reconcile.Result
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "identity",
			args: args{},
			want: false,
		},
		{
			name: "generic requeue takes precedence over no requeue",
			args: args{
				current: reconcile.Result{},
				next:    reconcile.Result{Requeue: true},
			},
			want: true,
		},
		{
			name: "shorter time to reconcile takes precedence",
			args: args{
				current: reconcile.Result{RequeueAfter: 1 * time.Hour},
				next:    reconcile.Result{RequeueAfter: 1 * time.Minute},
			},
			want: true,
		},
		{
			name: "specific requeue trumps generic requeue",
			args: args{
				current: reconcile.Result{Requeue: true},
				next:    reconcile.Result{RequeueAfter: 1 * time.Minute},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextResultTakesPrecedence(tt.args.current, tt.args.next); got != tt.want {
				t.Errorf("nextResultTakesPrecedence() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReconcileResults(t *testing.T) {
	tests := []struct {
		name string
		args []reconcile.Result
		want reconcile.Result
	}{
		{
			name: "none",
			args: nil,
			want: reconcile.Result{},
		},
		{
			name: "one",
			args: []reconcile.Result{{Requeue: true}},
			want: reconcile.Result{Requeue: true},
		},
		{
			name: "multiple",
			args: []reconcile.Result{{}, {Requeue: true}, {RequeueAfter: 1 * time.Second}},
			want: reconcile.Result{RequeueAfter: 1 * time.Second},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ReconcileResults{
				results: tt.args,
			}
			if got, _ := r.Aggregate(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Aggregate() = %v, want %v", got, tt.want)
			}
		})
	}
}

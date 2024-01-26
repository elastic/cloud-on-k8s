// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package reconcile

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"

	commonv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/shutdown"
)

func TestStatusReporter_MergeStatusReportingWith(t *testing.T) {
	type args struct {
		otherStatus esv1.ElasticsearchStatus
	}
	tests := []struct {
		name                    string
		state                   func() *State
		args                    args
		wantElasticsearchStatus esv1.ElasticsearchStatus
		wantPendingNewNodes     bool
	}{
		{
			name: "Happy path",
			state: func() *State {
				s := MustNewState(esv1.Elasticsearch{})
				// New nodes
				s.RecordNewNodes([]string{"new-0", "new-1", "new-2", "new-3"})
				s.UpdateNodesStatuses(esv1.NewNodePending, "new", "node 1 to 3 are delayed", 1, 3)
				// Nodes to be upgraded
				s.RecordNodesToBeUpgraded([]string{"to-upgrade-0", "to-upgrade-1", "to-upgrade-2"})
				s.RecordNodesToBeUpgradedWithMessage([]string{"to-upgrade-1"}, "An upgrade Message for to-upgrade-1")
				s.RecordDeletedNode("to-upgrade-2", "delete message")
				s.RecordPredicatesResult(map[string]string{"to-upgrade-0": "a-predicate-result"})
				// Nodes to be removed
				s.RecordNodesToBeRemoved([]string{"removed-0", "removed-1", "removed-2", "removed-3"})
				// removed-0 cannot be downscaled for now
				s.OnReconcileShutdowns([]string{"removed-1", "removed-2", "removed-3"})
				// removed-1 downscale is stalled
				s.OnShutdownStatus("removed-1", shutdown.NodeShutdownStatus{
					Status:      client.ShutdownStalled,
					Explanation: "stalled for a reason",
				})
				// removed-3 shutdown is complete
				s.OnShutdownStatus("removed-3", shutdown.NodeShutdownStatus{
					Status: client.ShutdownComplete,
				})
				s.ReportCondition(esv1.ElasticsearchIsReachable, corev1.ConditionFalse, "message1")
				s.ReportCondition(esv1.ReconciliationComplete, corev1.ConditionTrue, "initially reconciled")
				s.ReportCondition(esv1.ReconciliationComplete, corev1.ConditionFalse, "eventually not")
				return s
			},
			wantElasticsearchStatus: esv1.ElasticsearchStatus{
				Conditions: commonv1alpha1.Conditions{
					{
						Type:    "ElasticsearchIsReachable",
						Status:  "False",
						Message: "message1",
					},
					{
						Type:    "ReconciliationComplete",
						Status:  "False",
						Message: "eventually not",
					},
				},
				InProgressOperations: esv1.InProgressOperations{
					DownscaleOperation: esv1.DownscaleOperation{
						LastUpdatedTime: metav1.Time{},
						Nodes: []esv1.DownscaledNode{
							{
								Name:           "removed-0",
								ShutdownStatus: "NOT_STARTED",
								Explanation:    nil,
							},
							{
								Name:           "removed-1",
								ShutdownStatus: "STALLED",
								Explanation:    ptr.To[string]("stalled for a reason"),
							},
							{
								Name:           "removed-2",
								ShutdownStatus: "IN_PROGRESS",
								Explanation:    nil,
							},
							{
								Name:           "removed-3",
								ShutdownStatus: "COMPLETE",
								Explanation:    nil,
							},
						},
						Stalled: ptr.To[bool](true),
					},
					UpgradeOperation: esv1.UpgradeOperation{
						LastUpdatedTime: metav1.Time{},
						Nodes: []esv1.UpgradedNode{
							{
								Name:      "to-upgrade-0",
								Status:    "PENDING",
								Message:   ptr.To[string]("Cannot restart node because of failed predicate"),
								Predicate: ptr.To[string]("a-predicate-result"),
							},
							{
								Name:    "to-upgrade-1",
								Status:  "PENDING",
								Message: ptr.To[string]("An upgrade Message for to-upgrade-1"),
							},
							{
								Name:    "to-upgrade-2",
								Status:  "DELETED",
								Message: ptr.To[string]("delete message"),
							},
						},
					},
					UpscaleOperation: esv1.UpscaleOperation{
						LastUpdatedTime: metav1.Time{},
						Nodes: []esv1.NewNode{
							{Name: "new-0", Status: "PENDING", Message: ptr.To[string]("node 1 to 3 are delayed")},
							{Name: "new-1", Status: "PENDING", Message: ptr.To[string]("node 1 to 3 are delayed")},
							{Name: "new-2", Status: "PENDING", Message: ptr.To[string]("node 1 to 3 are delayed")},
							{Name: "new-3", Status: "PENDING"},
						},
					},
				},
			},
			wantPendingNewNodes: true, // we have pending nodes waiting to be created
		},
		{
			name: "Merge non-empty status",
			state: func() *State {
				s := MustNewState(esv1.Elasticsearch{})
				// New nodes
				s.RecordNewNodes([]string{"new-0"})
				// Nodes to be upgraded
				s.RecordNodesToBeUpgraded([]string{"to-upgrade-1", "to-upgrade-2"})
				s.RecordNodesToBeUpgradedWithMessage([]string{"to-upgrade-1"}, "An upgrade Message for to-upgrade-1")
				s.RecordDeletedNode("to-upgrade-2", "delete message")
				// Nodes to be removed
				s.RecordNodesToBeRemoved([]string{"removed-0", "removed-1", "removed-2"})
				// removed-0 cannot be downscaled for now
				s.OnReconcileShutdowns([]string{"removed-1", "removed-2"})
				s.ReportCondition(esv1.ElasticsearchIsReachable, corev1.ConditionFalse, "message1")
				s.ReportCondition(esv1.ReconciliationComplete, corev1.ConditionFalse, "eventually not")
				return s
			},
			args: args{
				otherStatus: esv1.ElasticsearchStatus{
					Phase: esv1.ElasticsearchResourceInvalid,
					Conditions: commonv1alpha1.Conditions{
						{
							Type:    "ReconciliationComplete",
							Status:  "True",
							Message: "initially reconciled",
						},
					},
					InProgressOperations: esv1.InProgressOperations{
						DownscaleOperation: esv1.DownscaleOperation{
							LastUpdatedTime: metav1.Time{},
							Nodes: []esv1.DownscaledNode{
								{
									Name:           "removed-1",
									ShutdownStatus: "STALLED",
									Explanation:    ptr.To[string]("stalled for a reason"),
								},
							},
							Stalled: ptr.To[bool](true),
						},
						UpgradeOperation: esv1.UpgradeOperation{
							LastUpdatedTime: metav1.Time{},
							Nodes: []esv1.UpgradedNode{
								{
									Name:      "to-upgrade-0",
									Status:    "PENDING",
									Message:   ptr.To[string]("Cannot restart node because of failed predicate"),
									Predicate: ptr.To[string]("a-predicate-result"),
								},
							},
						},
						UpscaleOperation: esv1.UpscaleOperation{
							LastUpdatedTime: metav1.Time{},
							Nodes: []esv1.NewNode{
								{Name: "new-1", Status: "PENDING", Message: ptr.To[string]("node 1 to 3 are delayed")},
								{Name: "new-2", Status: "PENDING", Message: ptr.To[string]("node 1 to 3 are delayed")},
								{Name: "new-3", Status: "PENDING"},
							},
						},
					},
				},
			},
			wantElasticsearchStatus: esv1.ElasticsearchStatus{
				Phase: esv1.ElasticsearchResourceInvalid,
				Conditions: commonv1alpha1.Conditions{
					{
						Type:    "ReconciliationComplete",
						Status:  "False",
						Message: "eventually not",
					},
					{
						Type:    "ElasticsearchIsReachable",
						Status:  "False",
						Message: "message1",
					},
				},
				InProgressOperations: esv1.InProgressOperations{
					DownscaleOperation: esv1.DownscaleOperation{
						LastUpdatedTime: metav1.Time{},
						Nodes: []esv1.DownscaledNode{
							{
								Name:           "removed-0",
								ShutdownStatus: "NOT_STARTED",
								Explanation:    nil,
							},
							{
								Name:           "removed-1",
								ShutdownStatus: "IN_PROGRESS",
								Explanation:    nil,
							},
							{
								Name:           "removed-2",
								ShutdownStatus: "IN_PROGRESS",
								Explanation:    nil,
							},
						},
						Stalled: nil,
					},
					UpgradeOperation: esv1.UpgradeOperation{
						LastUpdatedTime: metav1.Time{},
						Nodes: []esv1.UpgradedNode{
							{
								Name:    "to-upgrade-1",
								Status:  "PENDING",
								Message: ptr.To[string]("An upgrade Message for to-upgrade-1"),
							},
							{
								Name:    "to-upgrade-2",
								Status:  "DELETED",
								Message: ptr.To[string]("delete message"),
							},
						},
					},
					UpscaleOperation: esv1.UpscaleOperation{
						LastUpdatedTime: metav1.Time{},
						Nodes: []esv1.NewNode{
							{Name: "new-0", Status: "PENDING"},
						},
					},
				},
			},
			wantPendingNewNodes: true,
		},
		{
			name: "Merge empty status",
			state: func() *State {
				s := MustNewState(esv1.Elasticsearch{})
				return s
			},
			args: args{
				otherStatus: esv1.ElasticsearchStatus{},
			},
			wantElasticsearchStatus: esv1.ElasticsearchStatus{},
			wantPendingNewNodes:     false,
		},
		{
			name: "Merge empty status with nil slices of nodes",
			state: func() *State {
				s := MustNewState(esv1.Elasticsearch{})
				return s
			},
			args: args{
				otherStatus: esv1.ElasticsearchStatus{
					Phase: esv1.ElasticsearchResourceInvalid,
					Conditions: commonv1alpha1.Conditions{
						{
							Type:    "ReconciliationComplete",
							Status:  "True",
							Message: "initially reconciled",
						},
					},
					InProgressOperations: esv1.InProgressOperations{
						DownscaleOperation: esv1.DownscaleOperation{
							Nodes: nil,
						},
						UpgradeOperation: esv1.UpgradeOperation{
							Nodes: nil,
						},
						UpscaleOperation: esv1.UpscaleOperation{
							Nodes: nil,
						},
					},
				},
			},
			wantElasticsearchStatus: esv1.ElasticsearchStatus{
				Phase: esv1.ElasticsearchResourceInvalid,
				Conditions: commonv1alpha1.Conditions{
					{
						Type:    "ReconciliationComplete",
						Status:  "True",
						Message: "initially reconciled",
					},
				},
				InProgressOperations: esv1.InProgressOperations{
					DownscaleOperation: esv1.DownscaleOperation{
						Nodes: nil,
					},
					UpgradeOperation: esv1.UpgradeOperation{
						Nodes: nil,
					},
					UpscaleOperation: esv1.UpscaleOperation{
						Nodes: nil,
					},
				},
			},
			wantPendingNewNodes: false,
		},
		{
			name: "Merge empty status with empty slices of nodes",
			state: func() *State {
				s := MustNewState(esv1.Elasticsearch{})
				return s
			},
			args: args{
				otherStatus: esv1.ElasticsearchStatus{
					Phase: esv1.ElasticsearchResourceInvalid,
					Conditions: commonv1alpha1.Conditions{
						{
							Type:    "ReconciliationComplete",
							Status:  "True",
							Message: "initially reconciled",
						},
					},
					InProgressOperations: esv1.InProgressOperations{
						DownscaleOperation: esv1.DownscaleOperation{
							Nodes: []esv1.DownscaledNode{},
						},
						UpgradeOperation: esv1.UpgradeOperation{
							Nodes: []esv1.UpgradedNode{},
						},
						UpscaleOperation: esv1.UpscaleOperation{
							Nodes: []esv1.NewNode{},
						},
					},
				},
			},
			wantElasticsearchStatus: esv1.ElasticsearchStatus{
				Phase: esv1.ElasticsearchResourceInvalid,
				Conditions: commonv1alpha1.Conditions{
					{
						Type:    "ReconciliationComplete",
						Status:  "True",
						Message: "initially reconciled",
					},
				},
				InProgressOperations: esv1.InProgressOperations{
					DownscaleOperation: esv1.DownscaleOperation{
						Nodes: nil,
					},
					UpgradeOperation: esv1.UpgradeOperation{
						Nodes: nil,
					},
					UpscaleOperation: esv1.UpscaleOperation{
						Nodes: nil,
					},
				},
			},
			wantPendingNewNodes: false,
		},
		{
			name: "Do not overwrite existing status if reporter is not used",
			state: func() *State {
				s := MustNewState(esv1.Elasticsearch{})
				return s
			},
			args: args{
				otherStatus: esv1.ElasticsearchStatus{
					Phase: esv1.ElasticsearchResourceInvalid,
					Conditions: commonv1alpha1.Conditions{
						{
							Type:    "ReconciliationComplete",
							Status:  "False",
							Message: "Nodes upgrade in progress",
						},
					},
					InProgressOperations: esv1.InProgressOperations{
						DownscaleOperation: esv1.DownscaleOperation{
							Nodes: nil,
						},
						UpgradeOperation: esv1.UpgradeOperation{
							LastUpdatedTime: metav1.Now(),
							Nodes: []esv1.UpgradedNode{
								{
									Message:   ptr.To[string]("Cannot restart node because of failed predicate"),
									Name:      "node-1",
									Status:    "PENDING",
									Predicate: ptr.To[string]("if_yellow_only_restart_upgrading_nodes_with_unassigned_replicas"),
								},
							},
						},
						UpscaleOperation: esv1.UpscaleOperation{
							Nodes: nil,
						},
					},
				},
			},
			wantElasticsearchStatus: esv1.ElasticsearchStatus{
				Phase: esv1.ElasticsearchResourceInvalid,
				Conditions: commonv1alpha1.Conditions{
					{
						Type:    "ReconciliationComplete",
						Status:  "False",
						Message: "Nodes upgrade in progress",
					},
				},
				InProgressOperations: esv1.InProgressOperations{
					DownscaleOperation: esv1.DownscaleOperation{
						Nodes: nil,
					},
					UpgradeOperation: esv1.UpgradeOperation{
						Nodes: []esv1.UpgradedNode{
							{
								Message:   ptr.To[string]("Cannot restart node because of failed predicate"),
								Name:      "node-1",
								Status:    "PENDING",
								Predicate: ptr.To[string]("if_yellow_only_restart_upgrading_nodes_with_unassigned_replicas"),
							},
						},
					},
					UpscaleOperation: esv1.UpscaleOperation{
						Nodes: nil,
					},
				},
			},
			wantPendingNewNodes: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.state()
			// embed the status in Elasticsearch to use comparison.AssertEqual
			got := &esv1.Elasticsearch{Status: s.MergeStatusReportingWith(tt.args.otherStatus)}
			want := &esv1.Elasticsearch{Status: tt.wantElasticsearchStatus}
			comparison.AssertEqual(t, got, want)
			assert.Equal(t, tt.wantPendingNewNodes, s.HasPendingNewNodes())
		})
	}
}

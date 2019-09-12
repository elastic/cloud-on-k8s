// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestUpgradePodsDeletion_Delete(t *testing.T) {
	type fields struct {
		upgradeTestPods upgradeTestPods
		ES              v1alpha1.Elasticsearch
		green           bool
		maxUnavailable  int
		podFilter       filter
	}
	tests := []struct {
		name                         string
		fields                       fields
		deleted                      []string
		wantErr                      bool
		wantShardsAllocationDisabled bool
	}{
		{
			name: "Do not attempt to delete an already terminating Pod",
			fields: fields{
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("master-0").isMaster(true).isData(false).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("node-0").isMaster(false).isData(true).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("node-1").isMaster(false).isData(true).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("node-2").isMaster(false).isData(true).isHealthy(true).needsUpgrade(true).isInCluster(true).isTerminating(true),
				),
				maxUnavailable: 2,
				green:          true,
				podFilter:      nothing,
			},
			deleted:                      []string{"node-1"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			name: "All Pods are upgraded",
			fields: fields{
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masters-0").isMaster(true).isData(true).isHealthy(true).needsUpgrade(false).isInCluster(true),
					newTestPod("masters-1").isMaster(true).isData(true).isHealthy(true).needsUpgrade(false).isInCluster(true),
					newTestPod("masters-2").isMaster(true).isData(true).isHealthy(true).needsUpgrade(false).isInCluster(true),
				),
				maxUnavailable: 1,
				green:          true,
				podFilter:      nothing,
			},
			deleted:                      []string{},
			wantErr:                      false,
			wantShardsAllocationDisabled: false,
		},
		{
			name: "3 healthy master and data nodes, allow the last to be upgraded",
			fields: fields{
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masters-2").isMaster(true).isData(true).isHealthy(true).needsUpgrade(false).isInCluster(true),
					newTestPod("masters-1").isMaster(true).isData(true).isHealthy(true).needsUpgrade(false).isInCluster(true),
					newTestPod("masters-0").isMaster(true).isData(true).isHealthy(true).needsUpgrade(true).isInCluster(true),
				),
				maxUnavailable: 1,
				green:          true,
				podFilter:      nothing,
			},
			deleted:                      []string{"masters-0"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			name: "3 healthy masters, allow the deletion of 1",
			fields: fields{
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masters-2").isMaster(true).isData(true).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("masters-1").isMaster(true).isData(true).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("masters-0").isMaster(true).isData(true).isHealthy(true).needsUpgrade(true).isInCluster(true),
				),
				maxUnavailable: 1,
				green:          true,
				podFilter:      nothing,
			},
			deleted:                      []string{"masters-2"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			name: "3 healthy masters, allow the deletion of 1 even if maxUnavailable > 1",
			fields: fields{
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masters-2").isMaster(true).isData(true).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("masters-1").isMaster(true).isData(true).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("masters-0").isMaster(true).isData(true).isHealthy(true).needsUpgrade(true).isInCluster(true),
				),
				maxUnavailable: 2,
				green:          true,
				podFilter:      nothing,
			},
			deleted:                      []string{"masters-2"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			name: "2 healthy masters out of 3, allow the deletion of the unhealthy one",
			fields: fields{
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("master-0").isMaster(true).isData(true).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("master-1").isMaster(true).isData(true).isHealthy(false).needsUpgrade(true).isInCluster(false),
					newTestPod("master-2").isMaster(true).isData(true).isHealthy(true).needsUpgrade(true).isInCluster(true),
				),
				maxUnavailable: 1,
				green:          true,
				podFilter:      nothing,
			},
			deleted:                      []string{"master-1"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			name: "1 master and 1 data node, wait for the node to be upgraded first",
			fields: fields{
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("master-0").isMaster(true).isData(false).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("node-0").isMaster(false).isData(true).isHealthy(true).needsUpgrade(true).isInCluster(true),
				),
				maxUnavailable: 1,
				green:          true,
				podFilter:      nothing,
			},
			deleted:                      []string{"node-0"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			name: "Do not delete healthy node if not green",
			fields: fields{
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("master-0").isMaster(true).isData(true).isHealthy(true).needsUpgrade(true).isInCluster(true),
				),
				maxUnavailable: 1,
				green:          false,
				podFilter:      nothing,
			},
			deleted:                      []string{},
			wantErr:                      false,
			wantShardsAllocationDisabled: false,
		},
		{
			name: "Allow deletion of unhealthy node if not green",
			fields: fields{
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masters-0").isMaster(true).isData(true).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("masters-2").isMaster(true).isData(true).isHealthy(false).needsUpgrade(true).isInCluster(false),
					newTestPod("masters-1").isMaster(true).isData(true).isHealthy(true).needsUpgrade(true).isInCluster(true),
				),
				maxUnavailable: 1,
				green:          false,
				podFilter:      nothing,
			},
			deleted:                      []string{"masters-2"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			name: "Do not delete last healthy master",
			fields: fields{
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masters-0").isMaster(true).isData(true).isHealthy(true).needsUpgrade(false).isInCluster(true),
					newTestPod("masters-1").isMaster(true).isData(true).isHealthy(false).needsUpgrade(true).isInCluster(false),
				),
				maxUnavailable: 1,
				green:          true,
				podFilter:      nothing,
			},
			deleted:                      []string{"masters-1"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			name: "Pod deleted while upgrading",
			fields: fields{
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masters-0").isMaster(true).isData(true).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("masters-1").isMaster(true).isData(true).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("masters-2").isMaster(true).isData(true).isHealthy(true).needsUpgrade(true).isInCluster(true),
				),
				maxUnavailable: 1,
				green:          true,
				podFilter:      byName("masters-2"),
			},
			deleted:                      []string{},
			wantErr:                      true,
			wantShardsAllocationDisabled: true,
		},
		{
			// This test is relying on the cluster state to check if some shards (master and replica) are shared by
			// some nodes. The fake cluster state used in this test is in the testdata/cluster_state.json
			name: "Do not delete Pods that share some shards",
			fields: fields{
				upgradeTestPods: newUpgradeTestPods(
					// 5 data nodes
					newTestPod("elasticsearch-sample-es-nodes-4").isMaster(false).isData(true).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("elasticsearch-sample-es-nodes-3").isMaster(false).isData(true).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("elasticsearch-sample-es-nodes-2").isMaster(false).isData(true).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("elasticsearch-sample-es-nodes-1").isMaster(false).isData(true).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("elasticsearch-sample-es-nodes-0").isMaster(false).isData(true).isHealthy(true).needsUpgrade(true).isInCluster(true),
					// 3 masters
					newTestPod("elasticsearch-sample-es-masters-2").isMaster(true).isData(false).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("elasticsearch-sample-es-masters-1").isMaster(true).isData(false).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("elasticsearch-sample-es-masters-0").isMaster(true).isData(false).isHealthy(true).needsUpgrade(true).isInCluster(true),
				),
				maxUnavailable: 2, // Allow 2 to be upgraded at the same time
				green:          true,
				podFilter:      nothing,
			},
			// elasticsearch-sample-es-nodes-3 must be skipped because it shares a shard with elasticsearch-sample-es-nodes-4
			// elasticsearch-sample-es-nodes-2 can be deleted because 2 nodes are allowed to be unavailable and it does not share
			// some shards with elasticsearch-sample-es-nodes-4
			deleted:                      []string{"elasticsearch-sample-es-nodes-4", "elasticsearch-sample-es-nodes-2"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
	}
	for _, tt := range tests {
		esState := &testESState{
			inCluster: tt.fields.upgradeTestPods.podsInCluster(),
			green:     tt.fields.green,
		}
		esClient := &fakeESClient{}
		ctx := rollingUpgradeCtx{
			client: k8s.WrapClient(
				fake.NewFakeClient(tt.fields.upgradeTestPods.toPods(tt.fields.podFilter)...),
			),
			ES:              tt.fields.upgradeTestPods.toES(tt.fields.maxUnavailable),
			statefulSets:    tt.fields.upgradeTestPods.toStatefulSetList(),
			esClient:        esClient,
			esState:         esState,
			expectations:    expectations.NewExpectations(),
			expectedMasters: tt.fields.upgradeTestPods.toMasters(),
			podsToUpgrade:   tt.fields.upgradeTestPods.toUpgrade(),
			healthyPods:     tt.fields.upgradeTestPods.toHealthyPods(),
		}

		deleted, err := ctx.Delete()
		if (err != nil) != tt.wantErr {
			t.Errorf("runPredicates error = %v, wantErr %v", err, tt.wantErr)
			return
		}
		assert.ElementsMatch(t, names(deleted), tt.deleted, tt.name)
		assert.Equal(t, tt.wantShardsAllocationDisabled, esClient.DisableReplicaShardsAllocationCalled, tt.name)
	}
}

func TestDeletionStrategy_SortFunction(t *testing.T) {
	type fields struct {
		upgradeTestPods upgradeTestPods
		esState         ESState
	}
	tests := []struct {
		name   string
		fields fields
		want   []string // for this test we just compare the pod names
	}{
		{
			name: "Mixed nodes",
			fields: fields{
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masters-0").needsUpgrade(true),
					newTestPod("data-0").needsUpgrade(true),
					newTestPod("masters-1").needsUpgrade(true),
					newTestPod("data-1").needsUpgrade(true),
					newTestPod("masters-2").needsUpgrade(true),
				),
				esState: &testESState{
					inCluster: []string{"data-1", "data-0", "masters-2", "masters-1", "masters-0"},
					green:     false,
				},
			},
			want: []string{"data-1", "data-0", "masters-2", "masters-1", "masters-0"},
		},
		{
			name: "Masters first",
			fields: fields{
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masters-0").needsUpgrade(true),
					newTestPod("masters-1").needsUpgrade(true),
					newTestPod("masters-2").needsUpgrade(true),
					newTestPod("data-0").needsUpgrade(true),
					newTestPod("data-1").needsUpgrade(true),
					newTestPod("data-2").needsUpgrade(true),
				),
				esState: &testESState{
					inCluster: []string{"data-2", "data-1", "data-0", "masters-2", "masters-1", "masters-0"},
					green:     false,
				},
			},
			want: []string{"data-2", "data-1", "data-0", "masters-2", "masters-1", "masters-0"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toUpgrade := tt.fields.upgradeTestPods.toUpgrade()
			sortCandidates(toUpgrade)
			assert.Equal(t, len(tt.want), len(toUpgrade))
			for i := range tt.want {
				if tt.want[i] != toUpgrade[i].Name {
					t.Errorf("DeletionStrategyContext.SortFunction() = %v, want %v", names(toUpgrade), tt.want)
				}
			}
		})
	}
}

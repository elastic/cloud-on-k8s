// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/migration"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/shutdown"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func Test_hasDependencyInOthers(t *testing.T) {
	tests := []struct {
		name      string
		candidate esv1.ElasticsearchSettings
		other     []esv1.ElasticsearchSettings
		want      bool
	}{
		{
			candidate: newSettings(esv1.DataColdRole),
			other:     []esv1.ElasticsearchSettings{emptySettingsNode}, // all the roles, including master and frozen
			want:      false,                                           // other is a dependency but is also a master node
		},
		{
			candidate: newSettings(esv1.MasterRole, esv1.DataColdRole),
			other:     []esv1.ElasticsearchSettings{newSettings(esv1.MasterRole, esv1.DataFrozenRole)},
			want:      true, // cold depends on frozen
		},
		{
			candidate: newSettings(esv1.MasterRole, esv1.DataFrozenRole),
			other:     []esv1.ElasticsearchSettings{newSettings(esv1.MasterRole, esv1.DataColdRole)},
			want:      false, // frozen does not depend on cold
		},
		{
			candidate: newSettings(esv1.MasterRole),
			other:     []esv1.ElasticsearchSettings{newSettings(esv1.DataWarmRole)},
			want:      false, // master only nodes have no data tier dependency
		},
		{
			candidate: emptySettingsNode, // all the roles, including hot
			other:     []esv1.ElasticsearchSettings{newSettings(esv1.DataWarmRole)},
			want:      true, // hot depends on warm
		},
		{
			candidate: newSettings(esv1.DataFrozenRole),
			other:     []esv1.ElasticsearchSettings{newSettings(esv1.DataColdRole)},
			want:      false, // frozen does not depend on cold
		},
		{
			candidate: newSettings(esv1.DataHotRole),
			other:     []esv1.ElasticsearchSettings{newSettings(esv1.DataWarmRole)},
			want:      true, // hot does not depend on warm
		},
		{
			candidate: newSettings(esv1.DataFrozenRole, esv1.DataColdRole),
			other:     []esv1.ElasticsearchSettings{newSettings(esv1.DataColdRole, esv1.DataWarmRole)},
			want:      false, // frozen+cold does not depend on cold+warm
		},
		{
			candidate: newSettings(esv1.DataColdRole, esv1.DataWarmRole),
			other:     []esv1.ElasticsearchSettings{newSettings(esv1.DataColdRole)},
			want:      true, // warm depends on cold
		},
		{
			candidate: newSettings(esv1.DataFrozenRole, esv1.DataWarmRole),
			other:     []esv1.ElasticsearchSettings{newSettings(esv1.DataColdRole)},
			want:      false, // overlap: cold depends on frozen, but warm also depends on cold
		},
		{
			candidate: newSettings(esv1.DataFrozenRole, esv1.DataWarmRole),
			other:     []esv1.ElasticsearchSettings{newSettings(esv1.DataColdRole, esv1.DataHotRole)},
			want:      false, // same kind of overlap as above
		},
		{
			candidate: newSettings(esv1.DataFrozenRole, esv1.DataColdRole),
			other:     []esv1.ElasticsearchSettings{newSettings(esv1.DataWarmRole, esv1.DataHotRole)},
			want:      false, // frozen+cold can be upgraded before warm+hot
		},
		{
			candidate: newSettings(esv1.DataWarmRole, esv1.DataHotRole),
			other:     []esv1.ElasticsearchSettings{newSettings(esv1.DataFrozenRole, esv1.DataColdRole)},
			want:      true, // frozen+cold must be upgraded before warm+hot
		},
		{
			candidate: newSettings(esv1.DataColdRole, esv1.DataHotRole),
			other:     []esv1.ElasticsearchSettings{newSettings(esv1.DataFrozenRole, esv1.DataHotRole)},
			want:      false, // both nodes have hot role, cold can be upgraded before hot
		},
		{
			candidate: newSettings(esv1.DataFrozenRole, esv1.DataHotRole),
			other:     []esv1.ElasticsearchSettings{newSettings(esv1.DataColdRole, esv1.DataHotRole)},
			want:      false, // overlap: cold depends on frozen but hot depends on cold
		},
		{
			candidate: newSettings(esv1.DataColdRole, esv1.DataHotRole),
			other:     []esv1.ElasticsearchSettings{newSettings(esv1.DataRole)},
			want:      false, // overlap: cold depends on data because it includes frozen, but also includes warm which depends on cold
		},
		{
			candidate: newSettings(esv1.DataFrozenRole, esv1.DataHotRole),
			other:     []esv1.ElasticsearchSettings{newSettings(esv1.DataRole)},
			want:      false, // no strict dependency since data includes data_frozen
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasDependencyInOthers(tt.candidate, tt.other); got != tt.want {
				t.Errorf("roles.compare() = %v, want %v", got, tt.want)
			}
		})
	}
}

// These tests are focused on "type changes", i.e. when the type of a nodeSet is changed.
func TestUpgradePodsDeletion_WithNodeTypeMutations(t *testing.T) {
	type fields struct {
		esVersion       string
		upgradeTestPods upgradeTestPods
		health          client.Health
		mutation        mutation
		maxUnavailable  int
		esAnnotations   map[string]string
	}
	tests := []struct {
		name                         string
		fields                       fields
		deleted                      []string
		wantErr                      bool
		wantShardsAllocationDisabled bool
		/* Zen1 checks */
		minimumMasterNodesCalled     bool
		minimumMasterNodesCalledWith int
		recordedEvents               int
		/* Zend2 checks */
		votingExclusionCalledWith []string
	}{
		{
			// This unit test basically simulates the e2e test TestRiskyMasterReconfiguration.
			// It starts with 2 master+data nodes, the second one is changed to master only.
			name: "Risky mutation with 7.x nodes",
			fields: fields{
				esVersion: "7.2.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masterdata-0").withVersion("7.2.0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(false).isInCluster(true).inStatefulset("masterdata"),
					newTestPod("other-master-0").withVersion("7.2.0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true).inStatefulset("other-master"),
				),
				maxUnavailable: 1,
				health:         client.Health{Status: esv1.ElasticsearchGreenHealth},
				mutation:       removeMasterType("other-master"),
			},
			deleted:                      []string{"other-master-0"},
			votingExclusionCalledWith:    []string{"other-master-0"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			name: "Risky mutation with 6.x nodes",
			fields: fields{
				esVersion: "6.8.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masterdata-0").withVersion("6.8.0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(false).isInCluster(true),
					newTestPod("other-master-0").withVersion("6.8.0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
				),
				maxUnavailable: 1,
				health:         client.Health{Status: esv1.ElasticsearchGreenHealth},
				mutation:       removeMasterType("other-master"),
			},
			deleted:                      []string{"other-master-0"},
			minimumMasterNodesCalled:     true,
			minimumMasterNodesCalledWith: 1,
			recordedEvents:               1,
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			// Same test as above but the remaining master is unhealthy, nothing should be done
			name: "Risky mutation with an unhealthy master",
			fields: fields{
				esVersion: "7.2.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masterdata-0").withVersion("7.2.0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(false).needsUpgrade(false).isInCluster(true),
					newTestPod("other-master-0").withVersion("7.2.0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
				),
				maxUnavailable: 2, // 2 unavailable nodes to be sure that the predicate managing the masters is actually called
				health:         client.Health{Status: esv1.ElasticsearchGreenHealth},
				mutation:       removeMasterType("other-master"),
			},
			deleted:                      []string{},
			wantErr:                      false,
			wantShardsAllocationDisabled: false,
		},
		{
			name: "Two data nodes converted into master+data nodes, step 1: only 1 at a time is allowed",
			fields: fields{
				esVersion: "7.2.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masters-0").withVersion("7.2.0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(false).isInCluster(true),
					newTestPod("data-to-masters-0").withVersion("7.2.0").withRoles(esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("data-to-masters-1").withVersion("7.2.0").withRoles(esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
				),
				maxUnavailable: 2, // 2 unavailable nodes to be sure that the predicate managing the masters is actually called
				health:         client.Health{Status: esv1.ElasticsearchGreenHealth},
				mutation:       addMasterType("data-to-masters"),
			},
			deleted:                      []string{"data-to-masters-1"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			name: "Two data nodes converted into master+data nodes, step 2: upgrade the remaining one",
			fields: fields{
				esVersion: "7.2.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masters-0").withVersion("7.2.0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(false).isInCluster(true),
					newTestPod("data-to-masters-0").withVersion("7.2.0").withRoles(esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("data-to-masters-1").withVersion("7.2.0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(false).isInCluster(true),
				),
				maxUnavailable: 2, // 2 unavailable nodes to be sure that the predicate managing the masters is actually called
				health:         client.Health{Status: esv1.ElasticsearchGreenHealth},
				mutation:       addMasterType("data-to-masters"),
			},
			deleted:                      []string{"data-to-masters-0"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
	}
	for _, tt := range tests {
		esState := &testESState{
			inCluster: tt.fields.upgradeTestPods.podsInCluster(),
			health:    tt.fields.health,
		}
		esClient := &fakeESClient{version: version.MustParse("7.13.0")}
		es := tt.fields.upgradeTestPods.toES(tt.fields.esVersion, tt.fields.maxUnavailable, tt.fields.esAnnotations)
		k8sClient := k8s.NewFakeClient(tt.fields.upgradeTestPods.toRuntimeObjects(tt.fields.esVersion, tt.fields.maxUnavailable, nothing, tt.fields.esAnnotations)...)
		ctx := upgradeCtx{
			parentCtx:       context.Background(),
			client:          k8sClient,
			ES:              es,
			statefulSets:    tt.fields.upgradeTestPods.toStatefulSetList(),
			esClient:        esClient,
			shardLister:     migration.NewFakeShardLister(client.Shards{}),
			esState:         esState,
			expectations:    expectations.NewExpectations(k8sClient),
			reconcileState:  reconcile.MustNewState(esv1.Elasticsearch{}),
			expectedMasters: tt.fields.upgradeTestPods.toMasters(tt.fields.mutation),
			podsToUpgrade:   tt.fields.upgradeTestPods.toUpgrade(),
			healthyPods:     tt.fields.upgradeTestPods.toHealthyPods(),
			currentPods:     tt.fields.upgradeTestPods.toCurrentPods(),
		}

		deleted, err := ctx.Delete()
		if (err != nil) != tt.wantErr {
			t.Errorf("runPredicates error = %v, wantErr %v", err, tt.wantErr)
			return
		}
		assert.ElementsMatch(t, names(deleted), tt.deleted, tt.name)
		assert.Equal(t, tt.wantShardsAllocationDisabled, esClient.DisableReplicaShardsAllocationCalled, tt.name)
		/* Zen2 checks */
		assert.ElementsMatch(t, tt.votingExclusionCalledWith, esClient.AddVotingConfigExclusionsCalledWith, tt.name)
		/* Zen1 checks */
		assert.Equal(t, tt.minimumMasterNodesCalled, esClient.SetMinimumMasterNodesCalled, tt.name)
		assert.Equal(t, tt.minimumMasterNodesCalledWith, esClient.SetMinimumMasterNodesCalledWith, tt.name)
		assert.Equal(t, tt.recordedEvents, len(ctx.reconcileState.Events()), tt.name)
	}
}

func TestUpgradePodsDeletion_Delete(t *testing.T) {
	type fields struct {
		upgradeTestPods upgradeTestPods
		shardLister     client.ShardLister
		shutdowns       map[string]client.NodeShutdown
		health          client.Health
		maxUnavailable  int
		podFilter       filter
		esVersion       string
		esAnnotations   map[string]string
	}
	tests := []struct {
		name                         string
		fields                       fields
		deleted                      []string
		wantErr                      bool
		wantShardsAllocationDisabled bool
	}{
		{
			name: "multi tiers topology, data_frozen nodes must be upgraded first",
			fields: fields{
				esVersion: "7.15.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("master-0").withRoles(esv1.MasterRole).isHealthy(true).needsUpgrade(true).withVersion("7.14.0").isInCluster(true),
					newTestPod("cold-0").withRoles(esv1.DataColdRole).isHealthy(true).needsUpgrade(true).withVersion("7.14.0").isInCluster(true),
					newTestPod("frozen-0").withRoles(esv1.DataFrozenRole).isHealthy(true).needsUpgrade(true).withVersion("7.14.0").isInCluster(true),
					newTestPod("hot-warm-0").withRoles(esv1.DataWarmRole, esv1.DataHotRole).isHealthy(true).needsUpgrade(true).withVersion("7.14.0").isInCluster(true),
				),
				maxUnavailable: 1,
				shardLister:    migration.NewFakeShardLister(client.Shards{}),
				health:         client.Health{Status: esv1.ElasticsearchGreenHealth},
				podFilter:      nothing,
			},
			deleted:                      []string{"frozen-0"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			name: "multi tiers topology, data_hot nodes must be upgraded before last master with default roles",
			fields: fields{
				esVersion: "7.15.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("default-0").isHealthy(true).needsUpgrade(true).withVersion("7.14.0").isInCluster(true),
					newTestPod("default-1").isHealthy(true).needsUpgrade(false).withVersion("7.15.0").isInCluster(true),
					newTestPod("default-2").isHealthy(true).needsUpgrade(false).withVersion("7.15.0").isInCluster(true),
					newTestPod("cold-0").withRoles(esv1.DataColdRole).isHealthy(true).needsUpgrade(false).withVersion("7.15.0").isInCluster(true),
					newTestPod("hot-0").withRoles(esv1.DataHotRole).isHealthy(true).needsUpgrade(true).withVersion("7.14.0").isInCluster(true),
				),
				maxUnavailable: 1,
				shardLister:    migration.NewFakeShardLister(client.Shards{}),
				health:         client.Health{Status: esv1.ElasticsearchGreenHealth},
				podFilter:      nothing,
			},
			deleted:                      []string{"hot-0"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			name: "1 master and 2 data_content nodes, wait for all the data_content nodes to be upgraded first",
			fields: fields{
				esVersion: "7.15.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("master-0").withRoles(esv1.MasterRole).isHealthy(true).needsUpgrade(true).withVersion("7.14.0").isInCluster(true),
					newTestPod("node-0").withRoles(esv1.DataContentRole).isHealthy(true).needsUpgrade(true).withVersion("7.14.0").isInCluster(true),
					newTestPod("node-1").withRoles(esv1.DataContentRole).isHealthy(true).needsUpgrade(true).withVersion("7.14.0").isInCluster(true),
				),
				maxUnavailable: 2,
				shardLister: migration.NewFakeShardLister(client.Shards{
					client.Shard{
						Index:    "index_a",
						Shard:    "0",
						State:    "STARTED",
						NodeName: "node-0",
						Type:     "p",
					},
					client.Shard{
						Index:    "index_a",
						Shard:    "0",
						State:    "STARTED",
						NodeName: "node-1",
						Type:     "r",
					},
				}),
				health:    client.Health{Status: esv1.ElasticsearchGreenHealth},
				podFilter: nothing,
			},
			deleted:                      []string{"node-1"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			name: "Do not attempt to delete an already terminating Pod",
			fields: fields{
				esVersion: "7.5.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("master-0").withRoles(esv1.MasterRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("node-0").withRoles(esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("node-1").withRoles(esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("node-2").withRoles(esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true).isTerminating(true),
				),
				maxUnavailable: 2,
				shardLister:    migration.NewFakeShardLister(client.Shards{}),
				health:         client.Health{Status: esv1.ElasticsearchGreenHealth},
				podFilter:      nothing,
			},
			deleted:                      []string{"node-1"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			name: "All Pods are upgraded",
			fields: fields{
				esVersion: "7.5.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masters-0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(false).isInCluster(true),
					newTestPod("masters-1").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(false).isInCluster(true),
					newTestPod("masters-2").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(false).isInCluster(true),
				),
				maxUnavailable: 1,
				shardLister:    migration.NewFakeShardLister(client.Shards{}),
				health:         client.Health{Status: esv1.ElasticsearchGreenHealth},
				podFilter:      nothing,
			},
			deleted:                      []string{},
			wantErr:                      false,
			wantShardsAllocationDisabled: false,
		},
		{
			name: "3 healthy master and data nodes, allow the last to be upgraded",
			fields: fields{
				esVersion: "7.5.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masters-2").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(false).isInCluster(true),
					newTestPod("masters-1").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(false).isInCluster(true),
					newTestPod("masters-0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
				),
				maxUnavailable: 1,
				shardLister:    migration.NewFakeShardLister(client.Shards{}),
				health:         client.Health{Status: esv1.ElasticsearchGreenHealth},
				podFilter:      nothing,
			},
			deleted:                      []string{"masters-0"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			name: "3 healthy masters, allow the deletion of 1",
			fields: fields{
				esVersion: "7.5.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masters-2").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("masters-1").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("masters-0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
				),
				maxUnavailable: 1,
				shardLister:    migration.NewFakeShardLister(client.Shards{}),
				health:         client.Health{Status: esv1.ElasticsearchGreenHealth},
				podFilter:      nothing,
			},
			deleted:                      []string{"masters-2"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			name: "3 healthy masters, allow the deletion of 2 if 'one_master_at_a_time' predicate is disabled, and maxUnavailable == 2",
			fields: fields{
				esVersion: "7.5.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masters-2").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("masters-1").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("masters-0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
				),
				maxUnavailable: 2,
				shardLister:    migration.NewFakeShardLister(client.Shards{}),
				health:         client.Health{Status: esv1.ElasticsearchGreenHealth},
				podFilter:      nothing,
				esAnnotations: map[string]string{
					esv1.DisableUpgradePredicatesAnnotation: "one_master_at_a_time",
				},
			},
			deleted:                      []string{"masters-1", "masters-2"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			name: "3 unhealthy masters, red cluster, allow the deletion of all if all predicates are disabled, and maxUnavailable is 3 more than number of unhealthy pods",
			fields: fields{
				esVersion: "7.5.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masters-2").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(false).needsUpgrade(true).isInCluster(true),
					newTestPod("masters-1").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(false).needsUpgrade(true).isInCluster(true),
					newTestPod("masters-0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(false).needsUpgrade(true).isInCluster(true),
				),
				maxUnavailable: 6,
				shardLister:    migration.NewFakeShardLister(client.Shards{}),
				health:         client.Health{Status: esv1.ElasticsearchRedHealth},
				podFilter:      nothing,
				esAnnotations: map[string]string{
					esv1.DisableUpgradePredicatesAnnotation: "*",
				},
			},
			deleted:                      []string{"masters-0", "masters-1", "masters-2"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			name: "3 healthy masters, allow the deletion of 1 even if maxUnavailable > 1",
			fields: fields{
				esVersion: "7.5.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masters-2").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("masters-1").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("masters-0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
				),
				maxUnavailable: 2,
				shardLister:    migration.NewFakeShardLister(client.Shards{}),
				health:         client.Health{Status: esv1.ElasticsearchGreenHealth},
				podFilter:      nothing,
			},
			deleted:                      []string{"masters-2"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			name: "2 healthy masters out of 5, maxUnavailable is 2, allow the deletion of the unhealthy one",
			fields: fields{
				esVersion: "7.5.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("master-0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("master-1").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(false).needsUpgrade(true).isInCluster(false),
					newTestPod("master-2").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("master-3").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("master-4").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
				),
				maxUnavailable: 2,
				shardLister:    migration.NewFakeShardLister(client.Shards{}),
				health:         client.Health{Status: esv1.ElasticsearchGreenHealth},
				podFilter:      nothing,
			},
			deleted:                      []string{"master-1"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			name: "1 master and 1 data node, wait for the node to be upgraded first",
			fields: fields{
				esVersion: "7.5.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("master-0").withRoles(esv1.MasterRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("node-0").withRoles(esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
				),
				maxUnavailable: 1,
				shardLister:    migration.NewFakeShardLister(client.Shards{}),
				health:         client.Health{Status: esv1.ElasticsearchGreenHealth},
				podFilter:      nothing,
			},
			deleted:                      []string{"node-0"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			name: "Do not delete healthy node if red",
			fields: fields{
				esVersion: "7.5.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("master-0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
				),
				maxUnavailable: 1,
				shardLister:    migration.NewFakeShardLister(client.Shards{}),
				health:         client.Health{Status: esv1.ElasticsearchRedHealth},
				podFilter:      nothing,
			},
			deleted:                      []string{},
			wantErr:                      false,
			wantShardsAllocationDisabled: false,
		},
		{
			name: "Delete healthy node if red only with disabled predicate annotation",
			fields: fields{
				esVersion: "7.5.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("master-0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
				),
				maxUnavailable: 1,
				shardLister:    migration.NewFakeShardLister(client.Shards{}),
				health:         client.Health{Status: esv1.ElasticsearchRedHealth},
				podFilter:      nothing,
				esAnnotations: map[string]string{
					esv1.DisableUpgradePredicatesAnnotation: "only_restart_healthy_node_if_green_or_yellow",
				},
			},
			deleted:                      []string{"master-0"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			name: "Also delete healthy node if red with all predicates disabled via annotation",
			fields: fields{
				esVersion: "7.5.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("master-0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
				),
				maxUnavailable: 1,
				shardLister:    migration.NewFakeShardLister(client.Shards{}),
				health:         client.Health{Status: esv1.ElasticsearchRedHealth},
				podFilter:      nothing,
				esAnnotations: map[string]string{
					esv1.DisableUpgradePredicatesAnnotation: "*",
				},
			},
			deleted:                      []string{"master-0"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			name: "Do not delete healthy node if health is unknown",
			fields: fields{
				esVersion: "7.5.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("master-0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
				),
				maxUnavailable: 1,
				shardLister:    migration.NewFakeShardLister(client.Shards{}),
				health:         client.Health{Status: esv1.ElasticsearchUnknownHealth},
				podFilter:      nothing,
			},
			deleted:                      []string{},
			wantErr:                      false,
			wantShardsAllocationDisabled: false,
		},
		{
			name: "Allow deletion of unhealthy node if not green",
			fields: fields{
				esVersion: "7.5.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masters-0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("masters-2").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(false).needsUpgrade(true).isInCluster(false),
					newTestPod("masters-1").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
				),
				maxUnavailable: 1,
				shardLister:    migration.NewFakeShardLister(client.Shards{}),
				health:         client.Health{Status: esv1.ElasticsearchUnknownHealth},
				podFilter:      nothing,
			},
			deleted:                      []string{"masters-2"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			name: "Do not delete the node with the last remaining started shard, one of the nodes is unhealthy",
			fields: fields{
				esVersion: "7.5.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masters-0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true).withVersion("7.4.0"),
					newTestPod("masters-1").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(false).needsUpgrade(false).isInCluster(true).withVersion("7.5.0"),
				),
				maxUnavailable: 1,
				health:         client.Health{Status: esv1.ElasticsearchYellowHealth},
				shardLister: migration.NewFakeShardLister(client.Shards{
					// One shard is not assigned on masters-1 because the node is not healthy
					client.Shard{
						Index:    "index_a",
						Shard:    "0",
						State:    "STARTED",
						NodeName: "masters-0",
						Type:     "p",
					},
					client.Shard{
						Index:    "index_a",
						Shard:    "0",
						State:    "UNASSIGNED",
						NodeName: "",
						Type:     "r",
					},
				}),
				podFilter: nothing,
			},
			deleted:                      []string{},
			wantErr:                      false,
			wantShardsAllocationDisabled: false,
		},
		{
			name: "Allow deletion if yellow in a 2 node cluster and shard has replica unassigned",
			fields: fields{
				esVersion: "7.5.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masters-0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true).withVersion("7.4.0"),
					newTestPod("masters-1").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true).withVersion("7.4.0"),
				),
				maxUnavailable: 1,
				health:         client.Health{Status: esv1.ElasticsearchYellowHealth},
				shardLister: migration.NewFakeShardLister(client.Shards{
					client.Shard{
						Index:    "index_a",
						Shard:    "0",
						State:    "STARTED",
						NodeName: "masters-0",
						Type:     "p",
					},
					client.Shard{
						Index:    "index_a",
						Shard:    "0",
						State:    "UNASSIGNED",
						NodeName: "",
						Type:     "r",
					},
				}),
				podFilter: nothing,
			},
			deleted:                      []string{"masters-1"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			name: "Allow deletion if yellow in a single cluster node even if a shard has replica unassigned",
			fields: fields{
				esVersion: "7.5.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masters-0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true).withVersion("7.4.0"),
				),
				maxUnavailable: 1,
				health:         client.Health{Status: esv1.ElasticsearchYellowHealth},
				shardLister: migration.NewFakeShardLister(client.Shards{
					// One shard is not assigned because this is a single cluster node with default replica to 1
					client.Shard{
						Index:    "index_a",
						Shard:    "0",
						State:    "STARTED",
						NodeName: "masters-0",
						Type:     "p",
					},
					client.Shard{
						Index:    "index_a",
						Shard:    "0",
						State:    "UNASSIGNED",
						NodeName: "",
						Type:     "r",
					},
				}),
				podFilter: nothing,
			},
			deleted:                      []string{"masters-0"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			name: "Allow restart when the single node cluster is not been upgraded, rolling changes",
			fields: fields{
				esVersion: "7.5.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masters-0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true).withVersion("7.5.0"),
				),
				maxUnavailable: 1,
				health:         client.Health{Status: esv1.ElasticsearchYellowHealth},
				shardLister: migration.NewFakeShardLister(client.Shards{
					// One shard is not assigned because this is a single cluster node with default replica to 1
					client.Shard{
						Index:    "index_a",
						Shard:    "0",
						State:    "STARTED",
						NodeName: "masters-0",
						Type:     "p",
					},
					client.Shard{
						Index:    "index_a",
						Shard:    "0",
						State:    "UNASSIGNED",
						NodeName: "",
						Type:     "r",
					},
				}),
				podFilter: nothing,
			},
			deleted:                      []string{"masters-0"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			name: "Allow deletion of unhealthy node if yellow and node is not upgrading",
			fields: fields{
				esVersion: "7.5.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masters-0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true).withVersion("7.5.0"),
					newTestPod("masters-1").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true).withVersion("7.5.0"),
					newTestPod("masters-2").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(false).needsUpgrade(true).isInCluster(false).withVersion("7.5.0"),
				),
				maxUnavailable: 1,
				health:         client.Health{Status: esv1.ElasticsearchYellowHealth},
				shardLister: migration.NewFakeShardLister(client.Shards{
					client.Shard{
						Index:    "index_a",
						Shard:    "0",
						State:    "STARTED",
						NodeName: "masters-0",
						Type:     "p",
					},
					client.Shard{
						Index:    "index_a",
						Shard:    "0",
						State:    "UNASSIGNED",
						NodeName: "",
						Type:     "r",
					},
				}),
				podFilter: nothing,
			},
			deleted:                      []string{"masters-2"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			name: "Allow deletion of healthy node if yellow and node is upgrading",
			fields: fields{
				esVersion: "7.5.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masters-0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true).withVersion("7.4.0"),
					newTestPod("masters-1").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true).withVersion("7.4.0"),
					newTestPod("masters-2").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(false).isInCluster(true).withVersion("7.5.0"),
				),
				maxUnavailable: 1,
				health:         client.Health{Status: esv1.ElasticsearchYellowHealth},
				shardLister: migration.NewFakeShardLister(client.Shards{
					client.Shard{
						Index:    "index_a",
						Shard:    "0",
						State:    "STARTED",
						NodeName: "masters-0",
						Type:     "p",
					},
					client.Shard{
						Index:    "index_a",
						Shard:    "0",
						State:    "UNASSIGNED",
						NodeName: "",
						Type:     "r",
					},
				}),
				podFilter: nothing,
			},
			deleted:                      []string{"masters-1"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			name: "Do not delete the node with the last remaining started shard",
			fields: fields{
				esVersion: "7.5.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masters-0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true).withVersion("7.4.0"),
					newTestPod("masters-1").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true).withVersion("7.4.0"),
					newTestPod("masters-2").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(false).isInCluster(true).withVersion("7.5.0"),
					newTestPod("masters-3").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true).withVersion("7.4.0"),
					newTestPod("masters-4").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true).withVersion("7.4.0"),
				),
				maxUnavailable: 1,
				health:         client.Health{Status: esv1.ElasticsearchYellowHealth},
				shardLister: migration.NewFakeShardLister(client.Shards{
					client.Shard{
						Index:    "index_a",
						Shard:    "0",
						State:    "STARTED",
						NodeName: "masters-4",
						Type:     "p",
					},
					client.Shard{
						Index:    "index_a",
						Shard:    "0",
						State:    "UNASSIGNED",
						NodeName: "",
						Type:     "r",
					},
					client.Shard{
						Index:    "index_b",
						Shard:    "0",
						State:    "STARTED",
						NodeName: "masters-4",
						Type:     "r",
					},
					client.Shard{
						Index:    "index_b",
						Shard:    "0",
						State:    "STARTED",
						NodeName: "master-2",
						Type:     "p",
					},
				}),
				podFilter: nothing,
			},
			deleted:                      []string{"masters-3"}, // masters-4 must NOT be deleted because it is holding the last started primary shards
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			name: "Do not allow deletion of healthy node if yellow and all replica are unassigned",
			fields: fields{
				esVersion: "7.5.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masters-0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true).withVersion("7.4.0"),
					newTestPod("masters-1").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true).withVersion("7.4.0"),
					newTestPod("masters-2").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true).withVersion("7.5.0"),
				),
				maxUnavailable: 1,
				health:         client.Health{Status: esv1.ElasticsearchYellowHealth},
				shardLister: migration.NewFakeShardLister(client.Shards{
					client.Shard{
						Index:    "index_a",
						Shard:    "0",
						State:    "STARTED",
						NodeName: "masters-1",
						Type:     "p",
					},
					client.Shard{
						Index:    "index_a",
						Shard:    "0",
						State:    "UNASSIGNED",
						NodeName: "",
						Type:     "r",
					},
				}),
				podFilter: nothing,
			},
			deleted:                      []string{"masters-0"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			name: "Do not allow deletion if yellow if a shard is relocating",
			fields: fields{
				esVersion: "7.5.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masters-0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true).withVersion("7.4.0"),
					newTestPod("masters-1").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true).withVersion("7.4.0"),
					newTestPod("masters-2").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(false).isInCluster(true).withVersion("7.5.0"),
				),
				maxUnavailable: 1,
				health:         client.Health{Status: esv1.ElasticsearchYellowHealth, RelocatingShards: 1},
				shardLister: migration.NewFakeShardLister(client.Shards{
					client.Shard{
						Index:    "index_a",
						Shard:    "0",
						State:    "RELOCATING",
						NodeName: "masters-2",
						Type:     "p",
					},
				}),
				podFilter: nothing,
			},
			deleted:                      []string{},
			wantErr:                      false,
			wantShardsAllocationDisabled: false,
		},
		{
			name: "Allow deletion if yellow and if a shard has no replica",
			fields: fields{
				esVersion: "7.5.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masters-0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true).withVersion("7.4.0"),
					newTestPod("masters-1").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(false).isInCluster(true).withVersion("7.5.0"),
				),
				maxUnavailable: 1,
				health:         client.Health{Status: esv1.ElasticsearchYellowHealth},
				shardLister: migration.NewFakeShardLister(client.Shards{
					// One shard is not assigned on masters-0 because its version is not the expected one
					client.Shard{
						Index:    "index_a",
						Shard:    "0",
						State:    "UNASSIGNED",
						NodeName: "masters-0",
						Type:     "r",
					},
					// master-0 has the only shard for index_b, but since there are no replicas it should not prevent the deletion
					client.Shard{
						Index:    "index_b",
						Shard:    "0",
						State:    "STARTED",
						NodeName: "masters-0",
						Type:     "p",
					},
					client.Shard{
						Index:    "index_a",
						Shard:    "0",
						State:    "STARTED",
						NodeName: "masters-1",
						Type:     "p",
					},
				}),
				podFilter: nothing,
			},
			deleted:                      []string{"masters-0"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			name: "Do not delete last healthy master",
			fields: fields{
				esVersion: "7.5.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masters-0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(false).isInCluster(true),
					newTestPod("masters-1").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(false).needsUpgrade(true).isInCluster(false),
				),
				maxUnavailable: 1,
				shardLister:    migration.NewFakeShardLister(client.Shards{}),
				health:         client.Health{Status: esv1.ElasticsearchGreenHealth},
				podFilter:      nothing,
			},
			deleted:                      []string{"masters-1"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			name: "Do not delete all nodes of a tier",
			fields: fields{
				esVersion: "8.0.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("ingest-0").withRoles(esv1.IngestRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("ingest-ml-0").withRoles(esv1.IngestRole, esv1.MLRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("ingest-ml-1").withRoles(esv1.IngestRole, esv1.MLRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
				),
				shutdowns: map[string]client.NodeShutdown{
					"ingest-0":    {Status: client.ShutdownComplete},
					"ingest-ml-0": {Status: client.ShutdownComplete},
					"ingest-ml-1": {Status: client.ShutdownComplete},
				},
				maxUnavailable: 3,
				shardLister:    migration.NewFakeShardLister(client.Shards{}),
				health:         client.Health{Status: esv1.ElasticsearchGreenHealth},
				podFilter:      nothing,
			},
			deleted:                      []string{"ingest-0", "ingest-ml-1"},
			wantErr:                      false,
			wantShardsAllocationDisabled: false,
		},
		{
			name: "But allow deletion of single node tiers",
			fields: fields{
				esVersion: "8.0.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("ingest-0").withRoles(esv1.IngestRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
				),
				shutdowns: map[string]client.NodeShutdown{
					"ingest-0": {Status: client.ShutdownComplete},
				},
				maxUnavailable: 1,
				shardLister:    migration.NewFakeShardLister(client.Shards{}),
				health:         client.Health{Status: esv1.ElasticsearchGreenHealth},
				podFilter:      nothing,
			},
			deleted:                      []string{"ingest-0"},
			wantErr:                      false,
			wantShardsAllocationDisabled: false,
		},
		{
			name: "Do allow the deletion of unhealthy Pods in a tier",
			fields: fields{
				esVersion: "8.0.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("ingest-0").withRoles(esv1.IngestRole).isHealthy(false).needsUpgrade(true).isInCluster(true),
					newTestPod("ingest-1").withRoles(esv1.IngestRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
				),
				shutdowns: map[string]client.NodeShutdown{
					"ingest-0": {Status: client.ShutdownComplete},
					"ingest-1": {Status: client.ShutdownComplete},
				},
				maxUnavailable: 2,
				shardLister:    migration.NewFakeShardLister(client.Shards{}),
				health:         client.Health{Status: esv1.ElasticsearchGreenHealth},
				podFilter:      nothing,
			},
			deleted:                      []string{"ingest-0"},
			wantErr:                      false,
			wantShardsAllocationDisabled: false,
		},
		{
			name: "Pod deleted while upgrading",
			fields: fields{
				esVersion: "7.5.0",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masters-0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("masters-1").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("masters-2").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
				),
				maxUnavailable: 1,
				shardLister:    migration.NewFakeShardLister(client.Shards{}),
				health:         client.Health{Status: esv1.ElasticsearchGreenHealth},
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
				esVersion: "7.5.0",
				upgradeTestPods: newUpgradeTestPods(
					// 5 data nodes
					newTestPod("elasticsearch-sample-es-nodes-4").withRoles(esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("elasticsearch-sample-es-nodes-3").withRoles(esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("elasticsearch-sample-es-nodes-2").withRoles(esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("elasticsearch-sample-es-nodes-1").withRoles(esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("elasticsearch-sample-es-nodes-0").withRoles(esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
					// 3 masters
					newTestPod("elasticsearch-sample-es-masters-2").withRoles(esv1.MasterRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("elasticsearch-sample-es-masters-1").withRoles(esv1.MasterRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("elasticsearch-sample-es-masters-0").withRoles(esv1.MasterRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
				),
				shardLister:    migration.NewFakeShardFromFile("shards.json"),
				maxUnavailable: 2, // Allow 2 to be upgraded at the same time
				health:         client.Health{Status: esv1.ElasticsearchGreenHealth},
				podFilter:      nothing,
			},
			// elasticsearch-sample-es-nodes-3 must be skipped because it shares a shard with elasticsearch-sample-es-nodes-4
			// elasticsearch-sample-es-nodes-2 can be deleted because 2 nodes are allowed to be unavailable and it does not share
			// some shards with elasticsearch-sample-es-nodes-4
			deleted:                      []string{"elasticsearch-sample-es-nodes-4", "elasticsearch-sample-es-nodes-2"},
			wantErr:                      false,
			wantShardsAllocationDisabled: true,
		},
		{
			name: "Node shutdown API: predicates allow pod to be upgraded, but shutdown is not complete",
			fields: fields{
				esVersion: "7.15.2",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masters-0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
				),
				shutdowns: map[string]client.NodeShutdown{
					"masters-0": {Status: client.ShutdownInProgress},
				},
				maxUnavailable: 1,
				shardLister:    migration.NewFakeShardLister(client.Shards{}),
				health:         client.Health{Status: esv1.ElasticsearchGreenHealth},
				podFilter:      nothing,
			},
			deleted:                      []string{},
			wantErr:                      false,
			wantShardsAllocationDisabled: false,
		},
		{
			name: "Node shutdown API: predicates allow pod to be upgraded, but Pod not in cluster",
			fields: fields{
				esVersion: "7.15.2",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masters-1").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(false).isInCluster(true),
					newTestPod("masters-0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(false),
				),
				maxUnavailable: 1,
				shardLister:    migration.NewFakeShardLister(client.Shards{}),
				health:         client.Health{Status: esv1.ElasticsearchGreenHealth},
				podFilter:      nothing,
			},
			deleted:                      []string{},
			wantErr:                      true,
			wantShardsAllocationDisabled: false,
		},
		{
			name: "Node shutdown API: predicates allow pod to be upgraded, shutdown complete",
			fields: fields{
				esVersion: "7.15.2",
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("masters-0").withRoles(esv1.MasterRole, esv1.DataRole).isHealthy(true).needsUpgrade(true).isInCluster(true),
				),
				shutdowns: map[string]client.NodeShutdown{
					"masters-0": {Status: client.ShutdownComplete},
				},
				maxUnavailable: 1,
				shardLister:    migration.NewFakeShardLister(client.Shards{}),
				health:         client.Health{Status: esv1.ElasticsearchGreenHealth},
				podFilter:      nothing,
			},
			deleted:                      []string{"masters-0"},
			wantErr:                      false,
			wantShardsAllocationDisabled: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			esState := &testESState{
				inCluster: tt.fields.upgradeTestPods.podsInCluster(),
				health:    tt.fields.health,
			}
			esClient := &fakeESClient{version: version.MustParse(tt.fields.esVersion), Shutdowns: tt.fields.shutdowns}
			k8sClient := k8s.NewFakeClient(tt.fields.upgradeTestPods.toRuntimeObjects(tt.fields.esVersion, tt.fields.maxUnavailable, tt.fields.podFilter, tt.fields.esAnnotations)...)
			nodeShutdown := shutdown.NewNodeShutdown(esClient, tt.fields.upgradeTestPods.podNamesToESNodeID(), client.Restart, "", log)
			es := tt.fields.upgradeTestPods.toES(tt.fields.esVersion, tt.fields.maxUnavailable, tt.fields.esAnnotations)
			ctx := upgradeCtx{
				parentCtx:       context.Background(),
				reconcileState:  reconcile.MustNewState(es),
				client:          k8sClient,
				ES:              es,
				resourcesList:   tt.fields.upgradeTestPods.toResourcesList(t),
				statefulSets:    tt.fields.upgradeTestPods.toStatefulSetList(),
				esClient:        esClient,
				shardLister:     tt.fields.shardLister,
				esState:         esState,
				expectations:    expectations.NewExpectations(k8sClient),
				expectedMasters: tt.fields.upgradeTestPods.toMasters(noMutation),
				podsToUpgrade:   tt.fields.upgradeTestPods.toUpgrade(),
				healthyPods:     tt.fields.upgradeTestPods.toHealthyPods(),
				currentPods:     tt.fields.upgradeTestPods.toCurrentPods(),
				nodeShutdown:    nodeShutdown,
			}

			deleted, err := ctx.Delete()
			if (err != nil) != tt.wantErr {
				t.Errorf("runPredicates error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.ElementsMatch(t, names(deleted), tt.deleted, tt.name)
			assert.Equal(t, tt.wantShardsAllocationDisabled, esClient.DisableReplicaShardsAllocationCalled, tt.name)
		})
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
					// use "amasters" rather than "masters" to ensure we are not relying on the name sort accidentally
					newTestPod("amasters-0").withRoles(esv1.MasterRole).needsUpgrade(true),
					newTestPod("data-0").withRoles(esv1.DataRole).needsUpgrade(true),
					newTestPod("masters-0").withRoles(esv1.MasterRole).needsUpgrade(true),
					newTestPod("amasters-1").withRoles(esv1.MasterRole).needsUpgrade(true),
					newTestPod("data-1").withRoles(esv1.DataRole).needsUpgrade(true),
					newTestPod("amasters-2").withRoles(esv1.MasterRole).needsUpgrade(true),
				),
				esState: &testESState{
					inCluster: []string{"data-1", "data-0", "amasters-2", "amasters-1", "amasters-0", "masters-0"},
					health:    client.Health{Status: esv1.ElasticsearchUnknownHealth},
				},
			},
			want: []string{"data-1", "data-0", "amasters-2", "amasters-1", "amasters-0", "masters-0"},
		},
		{
			name: "Masters first",
			fields: fields{
				upgradeTestPods: newUpgradeTestPods(
					// use "amasters" rather than "masters" to ensure we are not relying on the name sort accidentally
					newTestPod("amasters-0").withRoles(esv1.MasterRole).needsUpgrade(true),
					newTestPod("amasters-1").withRoles(esv1.MasterRole).needsUpgrade(true),
					newTestPod("amasters-2").withRoles(esv1.MasterRole).needsUpgrade(true),
					newTestPod("data-0").withRoles(esv1.DataRole).needsUpgrade(true),
					newTestPod("data-1").withRoles(esv1.DataRole).needsUpgrade(true),
					newTestPod("data-2").withRoles(esv1.DataRole).needsUpgrade(true),
				),
				esState: &testESState{
					inCluster: []string{"data-2", "data-1", "data-0", "amasters-2", "amasters-1", "amasters-0"},
					health:    client.Health{Status: esv1.ElasticsearchUnknownHealth},
				},
			},
			want: []string{"data-2", "data-1", "data-0", "amasters-2", "amasters-1", "amasters-0"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toUpgrade := tt.fields.upgradeTestPods.toUpgrade()
			sortCandidates(toUpgrade)
			require.Equal(t, len(tt.want), len(toUpgrade))
			var actualNames []string
			for i := range toUpgrade {
				actualNames = append(actualNames, toUpgrade[i].Name)
			}
			assert.Equal(t, tt.want, actualNames)
		})
	}
}

func Test_groupByPredicates(t *testing.T) {
	type args struct {
		fp failedPredicates
	}
	tests := []struct {
		name string
		args args
		want map[string][]string
	}{
		{
			name: "Do not fail if Nil",
			args: args{fp: nil},
			want: map[string][]string{},
		},
		{
			name: "Do not fail if empty",
			args: args{fp: map[string]string{}},
			want: map[string][]string{},
		},
		{
			name: "Simple test",
			args: args{fp: map[string]string{
				"pod-0": "do_not_restart_healthy_node_if_MaxUnavailable_reached",
				"pod-1": "do_not_restart_healthy_node_if_MaxUnavailable_reached",
				"pod-3": "skip_already_terminating_pods",
			},
			},
			want: map[string][]string{
				"do_not_restart_healthy_node_if_MaxUnavailable_reached": {"pod-0", "pod-1"},
				"skip_already_terminating_pods":                         {"pod-3"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := groupByPredicates(tt.args.fp); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("groupByPredicates() = %v, want %v", got, tt.want)
			}
		})
	}
}

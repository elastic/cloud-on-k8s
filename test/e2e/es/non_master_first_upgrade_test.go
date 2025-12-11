// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build es || e2e

package es

import (
	"fmt"
	"testing"
	"time"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	essset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
)

// newNonMasterFirstUpgradeWatcher creates a watcher that monitors StatefulSet upgrade order
// and ensures non-master StatefulSets upgrade before master StatefulSets
func newNonMasterFirstUpgradeWatcher(es esv1.Elasticsearch) test.Watcher {
	var violations []string

	return test.NewWatcher(
		"watch StatefulSet upgrade order: non-master StatefulSets should upgrade before master StatefulSets",
		2*time.Second,
		func(k *test.K8sClient, t *testing.T) {
			statefulSets, err := essset.RetrieveActualStatefulSets(k.Client, k8s.ExtractNamespacedName(&es))
			if err != nil {
				t.Logf("failed to get StatefulSets: %s", err.Error())
				return
			}

			// Check if any master StatefulSet has its version higher than any non-master StatefulSet
			// which indicates that the master StatefulSet is upgrading before the non-master StatefulSets
			for _, sset := range statefulSets {
				masterSTSVersion, err := essset.GetESVersion(sset)
				if err != nil {
					t.Logf("failed to get StatefulSet version: %s", err.Error())
					continue
				}
				if !label.IsMasterNodeSet(sset) {
					continue
				}
				// Ensure that the master StatefulSet never has a version higher than any non-master StatefulSet.
				for _, otherSset := range statefulSets {
					// don't compare master against master.
					if label.IsMasterNodeSet(otherSset) {
						continue
					}
					otherSsetVersion, err := essset.GetESVersion(otherSset)
					if err != nil {
						t.Logf("failed to get StatefulSet version: %s", err.Error())
						continue
					}
					if masterSTSVersion.GT(otherSsetVersion) {
						violations = append(violations, fmt.Sprintf("master StatefulSet %s has a version higher than non-master StatefulSet %s", sset.Name, otherSset.Name))
					}
				}
			}
		},
		func(k *test.K8sClient, t *testing.T) {
			if len(violations) > 0 {
				t.Errorf("%d non-master first upgrade order violations detected", len(violations))
			}
		})
}

// runNonMasterFirstUpgradeTest runs the complete test for non-master first upgrade behavior
func runNonMasterFirstUpgradeTest(t *testing.T, initial, mutated elasticsearch.Builder) {
	watcher := newNonMasterFirstUpgradeWatcher(initial.Elasticsearch)

	test.RunMutationsWhileWatching(
		t,
		[]test.Builder{initial},
		[]test.Builder{mutated},
		[]test.Watcher{watcher},
	)
}

// TestNonMasterFirstUpgradeComplexTopology tests the non-master first upgrade behavior with a complex topology
func TestNonMasterFirstUpgradeComplexTopology(t *testing.T) {
	srcVersion, dstVersion := test.GetUpgradePathTo8x(test.Ctx().ElasticStackVersion)

	test.SkipInvalidUpgrade(t, srcVersion, dstVersion)

	initial := elasticsearch.NewBuilder("test-non-master-first-complex").
		WithVersion(srcVersion).
		WithESMasterNodes(3, elasticsearch.DefaultResources).
		WithESDataNodes(2, elasticsearch.DefaultResources).
		WithESCoordinatingNodes(1, elasticsearch.DefaultResources)

	mutated := initial.WithVersion(dstVersion).WithMutatedFrom(&initial)

	runNonMasterFirstUpgradeTest(t, initial, mutated)
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
	"context"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"

	sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	es_sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

// expectationsSatisfied checks that resources in our local cache match what we expect.
// If not, it's safer to not move on with StatefulSets and Pods reconciliation.
// Continuing with the reconciliation at this point may lead to:
// - calling ES orchestration settings (zen2/allocation excludes) with wrong assumptions
// (eg. incorrect number of nodes or master-eligible nodes topology)
// - create or delete more than one master node at once
func (d *Driver) expectationsSatisfied(ctx context.Context) (bool, string, error) {
	return d.expectationsSatisfiedExcept(ctx, false)
}

// nodeSpecsExpectationsSatisfied is expectationsSatisfied, except that it tolerates the leaving Pods that could not be
// created, for example because of a resource quota, see uncreatedLeavingPods: otherwise it could block the downscale
// that removes them forever. The rolling upgrades still use expectationsSatisfied.
func (d *Driver) nodeSpecsExpectationsSatisfied(ctx context.Context) (bool, string, error) {
	return d.expectationsSatisfiedExcept(ctx, true)
}

// expectationsSatisfiedExcept is expectationsSatisfied, except that the pending creations of a StatefulSet that has no
// pending deletions are ignored if tolerateUncreatedLeaving is true and they are uncreatedLeavingPods.
func (d *Driver) expectationsSatisfiedExcept(ctx context.Context, tolerateUncreatedLeaving bool) (bool, string, error) {
	log := ulog.FromContext(ctx)
	// make sure the cache is up-to-date
	expectationsOK, reason, err := d.Expectations.Satisfied()
	if err != nil {
		return false, "", err
	}
	if !expectationsOK {
		log.V(1).Info("Cache expectations are not satisfied yet, re-queueing", "namespace", d.ES.Namespace, "es_name", d.ES.Name, "reason", reason)
		return false, reason, nil
	}
	actualStatefulSets, err := es_sset.RetrieveActualStatefulSets(d.Client, k8s.ExtractNamespacedName(&d.ES))
	if err != nil {
		return false, "", err
	}
	// make sure StatefulSet statuses have been reconciled by the StatefulSet controller
	pendingStatefulSetReconciliation := actualStatefulSets.PendingReconciliation()
	if len(pendingStatefulSetReconciliation) > 0 {
		log.V(1).Info("StatefulSets observedGeneration is not reconciled yet, re-queueing", "namespace", d.ES.Namespace, "es_name", d.ES.Name)
		return false, fmt.Sprintf("observedGeneration is not reconciled yet for StatefulSets %s", strings.Join(pendingStatefulSetReconciliation.Names().AsSlice(), ",")), nil
	}
	// make sure pods have been reconciled by the StatefulSet controller
	for _, statefulSet := range actualStatefulSets {
		pendingCreations, pendingDeletions, err := sset.PendingPodsForStatefulSet(d.Client, statefulSet, label.StatefulSetNameLabelName)
		if err != nil {
			return false, "", err
		}
		if tolerateUncreatedLeaving && len(pendingCreations) > 0 && len(pendingDeletions) == 0 &&
			d.uncreatedLeavingPods(ctx, statefulSet, pendingCreations) {
			log.V(1).Info("Tolerating Pods that were not created, as the spec no longer includes them",
				"namespace", d.ES.Namespace, "es_name", d.ES.Name, "statefulset_name", statefulSet.Name, "pods", pendingCreations)
			continue
		}
		if ok, reason := sset.PendingPodsReconciled(ctx, statefulSet, pendingCreations, pendingDeletions); !ok {
			return false, reason, nil
		}
	}
	return true, "", nil
}

// uncreatedLeavingPods returns true if the given pending creations of the given StatefulSet are leaving according to
// the spec, and the StatefulSet controller did not create them either: its status must count as many Pods as the cache,
// so that a created Pod that the cache does not see yet is not tolerated.
func (d *Driver) uncreatedLeavingPods(ctx context.Context, statefulSet appsv1.StatefulSet, pendingCreations []string) bool {
	if statefulSet.Status.Replicas != sset.GetReplicas(statefulSet)-int32(len(pendingCreations)) { //nolint:gosec // G115: Pod count cannot realistically overflow int32
		return false
	}
	return desiredLeavingNodeNamesFromSpec(ctx, d.ES, es_sset.StatefulSetList{statefulSet}).HasAll(pendingCreations...)
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

func (d *defaultDriver) expectationsMet(actualStatefulSets sset.StatefulSetList) (bool, error) {
	if !d.Expectations.SatisfiedGenerations(actualStatefulSets.ObjectMetas()...) {
		// Our cache of StatefulSets is out of date compared to previous reconciliation operations.
		// Continuing with the reconciliation at this point may lead to:
		// - errors on rejected sset updates (conflict since cached resource out of date): that's ok
		// - calling ES orchestration settings (zen1/zen2/allocation excludes) with wrong assumptions: that's not ok
		// Hence we choose to abort the reconciliation early: will run again later with an updated cache.
		log.V(1).Info("StatefulSet cache out-of-date, re-queueing", "namespace", d.ES.Namespace, "es_name", d.ES.Name)
		return false, nil
	}

	podsReconciled, err := actualStatefulSets.PodReconciliationDone(d.Client)
	if err != nil {
		return false, err
	}
	if !podsReconciled {
		// Pods we have in the cache do not match StatefulSets we have in the cache.
		// This can happen if some pods have been scheduled for creation/deletion/upgrade
		// but the operation has not happened or been observed yet.
		// Continuing with nodes reconciliation at this point would be dangerous, since
		// we may update ES orchestration settings (zen1/zen2/allocation excludes) with
		// wrong assumptions (especially on master-eligible and ES version mismatches).
		return false, nil
	}

	// The last step here is to check if some Pods are being deleted.
	// We should wait for them to be recreated after a rolling upgrade.
	return d.Expectations.SatisfiedDeletions(k8s.ExtractNamespacedName(&d.ES), d)
}

func (d *defaultDriver) CanRemoveExpectation(podName types.NamespacedName, uid types.UID) (bool, error) {
	// Try to get the Pod
	var currentPod corev1.Pod
	err := d.Client.Get(podName, &currentPod)
	if err != nil {
		if errors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
	return currentPod.UID != uid, nil
}

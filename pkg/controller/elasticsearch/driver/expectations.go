// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"fmt"
	"strings"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// expectationsSatisfied checks that resources in our local cache match what we expect.
// If not, it's safer to not move on with StatefulSets and Pods reconciliation.
// Continuing with the reconciliation at this point may lead to:
// - calling ES orchestration settings (zen1/zen2/allocation excludes) with wrong assumptions
// (eg. incorrect number of nodes or master-eligible nodes topology)
// - create or delete more than one master node at once
func (d *defaultDriver) expectationsSatisfied() (bool, string, error) {
	// make sure the cache is up-to-date
	expectationsOK, reason, err := d.Expectations.Satisfied()
	if err != nil {
		return false, "", err
	}
	if !expectationsOK {
		log.V(1).Info("Cache expectations are not satisfied yet, re-queueing", "namespace", d.ES.Namespace, "es_name", d.ES.Name, "reason", reason)
		return false, reason, nil
	}
	actualStatefulSets, err := sset.RetrieveActualStatefulSets(d.Client, k8s.ExtractNamespacedName(&d.ES))
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
	return actualStatefulSets.PodReconciliationDone(d.Client)
}

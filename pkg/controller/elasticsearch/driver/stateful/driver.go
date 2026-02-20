// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// Package stateful implements the stateful Elasticsearch driver using StatefulSets.
package stateful

import (
	"context"

	corev1 "k8s.io/api/core/v1"

	commondriver "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/driver"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/driver/shared"
)

// Driver is the stateful Elasticsearch driver implementation using StatefulSets.
type Driver struct {
	driver.BaseDriver
}

// NewDriver returns a new stateful driver implementation.
func NewDriver(parameters driver.Parameters) driver.Driver {
	return &Driver{BaseDriver: driver.BaseDriver{Parameters: parameters}}
}

var _ commondriver.Interface = (*Driver)(nil)

// Reconcile fulfills the Driver interface and reconciles the cluster resources.
func (d *Driver) Reconcile(ctx context.Context) *reconciler.Results {
	// Reconcile resources which are common to all drivers.
	shared, results := shared.ReconcileSharedResources(ctx, d, d.Parameters)
	if results.HasError() {
		return results
	}
	defer shared.ESClient.Close()

	// Stateful specific: Service accounts hint
	results.WithError(d.maybeSetServiceAccountsOrchestrationHint(
		ctx, shared.ESReachable, shared.ESClient, shared.ResourcesState))

	// Stateful specific: Suspended pods
	// We want to reconcile suspended Pods before we start reconciling node specs as this is considered a debugging and
	// troubleshooting tool that does not follow the change budget restrictions
	if err := reconcileSuspendedPods(ctx, d.Client, d.ES, d.Expectations); err != nil {
		return results.WithError(err)
	}

	// Stateful specific: Node specs (StatefulSets, upgrades, downscales)
	return results.WithResults(d.reconcileNodeSpecs(
		ctx, shared.ESReachable, shared.ESClient, d.ReconcileState,
		*shared.ResourcesState, shared.KeystoreResources, shared.Meta))
}

// names returns the names of the given pods.
func names(pods []corev1.Pod) []string {
	result := make([]string, len(pods))
	for i, pod := range pods {
		result[i] = pod.Name
	}
	return result
}

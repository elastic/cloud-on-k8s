// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateless

import (
	"context"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	drivercommon "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/driver/common"
)

type statelessDriver struct {
	*drivercommon.DefaultDriverParameters
}

// NewDriver returns the stateful driver implementation.
func NewDriver(parameters *drivercommon.DefaultDriverParameters) drivercommon.Driver {
	return &statelessDriver{DefaultDriverParameters: parameters}
}

// Reconcile fulfills the Driver interface and reconciles the cluster resources for a stateless Elasticsearch cluster.
func (sd *statelessDriver) Reconcile(ctx context.Context) *reconciler.Results {
	results := reconciler.NewResult(ctx)

	// Reconcile common resources shared by all drivers (stateful/stateless)
	defaultDriverResult := sd.DefaultDriverParameters.Reconcile(ctx)
	defer defaultDriverResult.Close()
	results.WithResults(defaultDriverResult.Results)
	if results.HasError() {
		return results
	}

	// reconcile CloneSets and nodes configuration
	return results.WithResults(sd.reconcileTiers(ctx, sd.Expectations, defaultDriverResult.Meta))
}

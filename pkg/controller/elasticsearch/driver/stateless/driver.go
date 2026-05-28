// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// Package stateless implements the stateless Elasticsearch driver.
package stateless

import (
	"context"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	commondriver "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/driver"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/driver/shared"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/filesettings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/stackconfig"
)

// Driver is the stateless Elasticsearch driver implementation.
type Driver struct {
	driver.BaseDriver
}

// NewDriver returns a new stateless driver implementation.
func NewDriver(parameters driver.Parameters) driver.Driver {
	return &Driver{BaseDriver: driver.BaseDriver{Parameters: parameters}}
}

var _ commondriver.Interface = (*Driver)(nil)

// Reconcile fulfills the Driver interface and reconciles the cluster resources.
func (d *Driver) Reconcile(ctx context.Context) *reconciler.Results {
	// Reconcile resources which are common to all drivers.
	// clientAuthenticationRequired is always false for stateless: mTLS is rejected by validation.
	// TODO(#9204): wire up StackConfigPolicy support for stateless ES (currently passing empty PolicyConfig{}
	sharedState, results := shared.ReconcileSharedResources(ctx, d, d.Parameters, false, stackconfig.PolicyConfig{})
	if results.HasError() {
		return results
	}
	defer sharedState.ESClient.Close()

	// Stateless: secure settings are delivered via cluster_secrets in file-based settings
	// instead of the keystore init container used by stateful clusters.
	if err := d.reconcileSecureSettings(ctx); err != nil {
		return results.WithError(err)
	}

	// TODO(#9204): stateless-specific reconciliation (Deployments, tiers) will go here.

	return results
}

// reconcileSecureSettings builds cluster_secrets from all secure settings sources and
// reconciles them into the file settings Secret.
func (d *Driver) reconcileSecureSettings(ctx context.Context) error {
	clusterSecrets, err := d.buildClusterSecrets(ctx)
	if err != nil {
		return err
	}
	return filesettings.ReconcileClusterSecrets(ctx, d.Client, d.ES, clusterSecrets)
}

// buildClusterSecrets gathers all secure settings sources and returns them in the nested
// structure expected by Elasticsearch file-based settings cluster_secrets.
func (d *Driver) buildClusterSecrets(ctx context.Context) (*commonv1.Config, error) {
	return shared.BuildClusterSecrets(ctx, d.Client, d.Recorder(), d.DynamicWatches(), d.ES)
}

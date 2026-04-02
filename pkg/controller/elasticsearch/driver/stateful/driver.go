// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// Package stateful implements the stateful Elasticsearch driver using StatefulSets.
package stateful

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	commondriver "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/driver"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/driver/shared"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/filesettings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/securitycontext"
	remotekeystore "github.com/elastic/cloud-on-k8s/v3/pkg/controller/remotecluster/keystore"
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
	results := reconciler.NewResult(ctx)

	enterpriseFeaturesEnabled, err := d.LicenseChecker.EnterpriseFeaturesEnabled(ctx)
	if err != nil {
		return results.WithError(err)
	}

	// Resolve configuration first. This computes merged configs for all NodeSets
	// (including StackConfigPolicy) and detects clientAuthenticationRequired early,
	// before we create the ES client.
	resolvedConfig, err := ResolveConfig(ctx, d.Client, d.ES, d.OperatorParameters.IPFamily, enterpriseFeaturesEnabled)
	if err != nil {
		return results.WithError(err)
	}

	if resolvedConfig.ClientAuthenticationOverrideWarning != "" {
		d.ReconcileState.AddEvent(corev1.EventTypeWarning, events.EventReasonValidation, events.EventActionValidation, resolvedConfig.ClientAuthenticationOverrideWarning)
	}

	// Reconcile resources which are common to all drivers.
	sharedState, sharedResults := shared.ReconcileSharedResources(ctx, d, d.Parameters, resolvedConfig.ClientAuthenticationRequired)
	if sharedResults.HasError() {
		return results.WithResults(sharedResults)
	}
	defer sharedState.ESClient.Close()
	results.WithResults(sharedResults)

	// File settings (stateful: SCP-deferred empty secret creation)
	if d.Version.GTE(filesettings.FileBasedSettingsMinPreVersion) {
		requeue, err := shared.MaybeReconcileEmptyFileSettingsSecret(ctx, d.Client, d.LicenseChecker, &d.ES, d.OperatorParameters.OperatorNamespace)
		if err != nil {
			return results.WithError(err)
		} else if requeue {
			results.WithReconciliationState(
				shared.DefaultRequeue.WithReason(
					fmt.Sprintf("This cluster is targeted by at least one StackConfigPolicy, expecting Secret %s to be created by StackConfigPolicy controller",
						esv1.FileSettingsSecretName(d.ES.Name)),
				),
			)
		}
	}

	// Keystore (stateful: init container + volume)
	keystoreResources, err := d.reconcileKeystore(ctx, sharedState.Meta)
	if err != nil {
		return results.WithError(err)
	}

	// Stateful specific: Service accounts hint
	results.WithError(d.maybeSetServiceAccountsOrchestrationHint(
		ctx, sharedState.ESReachable, sharedState.ESClient, sharedState.ResourcesState))

	// Stateful specific: Suspended pods
	// We want to reconcile suspended Pods before we start reconciling node specs as this is considered a debugging and
	// troubleshooting tool that does not follow the change budget restrictions
	if err := reconcileSuspendedPods(ctx, d.Client, d.ES, d.Expectations); err != nil {
		return results.WithError(err)
	}

	// Stateful specific: Node specs (StatefulSets, upgrades, downscales)
	return results.WithResults(d.reconcileNodeSpecs(
		ctx, sharedState.ESReachable, sharedState.ESClient, d.ReconcileState,
		*sharedState.ResourcesState, keystoreResources, sharedState.Meta, resolvedConfig))
}

// reconcileKeystore reconciles the keystore init container and volume for stateful Elasticsearch.
func (d *Driver) reconcileKeystore(ctx context.Context, meta metadata.Metadata) (*keystore.Resources, error) {
	keystoreParams := initcontainer.KeystoreParams
	keystoreSecurityContext := securitycontext.For(d.Version, true)
	keystoreParams.SecurityContext = &keystoreSecurityContext

	remoteClusterAPIKeys, err := remotekeystore.APIKeySecretSource(ctx, &d.ES, d.Client)
	if err != nil {
		return nil, err
	}
	return keystore.ReconcileResources(ctx, d, &d.ES, esv1.ESNamer, meta, keystoreParams, remoteClusterAPIKeys...)
}

// names returns the names of the given pods.
func names(pods []corev1.Pod) []string {
	result := make([]string, len(pods))
	for i, pod := range pods {
		result[i] = pod.Name
	}
	return result
}

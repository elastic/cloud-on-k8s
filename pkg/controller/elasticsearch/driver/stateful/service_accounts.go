// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
	"context"

	"k8s.io/utils/ptr"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/bootstrap"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/hints"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/user"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/optional"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/set"
)

// maybeSetServiceAccountsOrchestrationHint attempts to update an orchestration hint to let the association controllers
// know whether all the nodes in the cluster are ready to authenticate service accounts.
func (d *Driver) maybeSetServiceAccountsOrchestrationHint(
	ctx context.Context,
	esReachable bool,
	securityClient esclient.SecurityClient,
	resourcesState *reconcile.ResourcesState,
) error {
	if d.ReconcileState.OrchestrationHints().ServiceAccounts.IsTrue() {
		// Orchestration hint is already set to true, there is no point going back to false.
		return nil
	}

	// Case 1: New cluster, we can immediately set the orchestration hint.
	if !bootstrap.AnnotatedForBootstrap(d.ES) {
		allNodesRunningServiceAccounts, err := esv1.AreServiceAccountsSupported(d.ES.Spec.Version)
		if err != nil {
			return err
		}
		d.ReconcileState.UpdateOrchestrationHints(
			d.ReconcileState.OrchestrationHints().Merge(hints.OrchestrationsHints{ServiceAccounts: optional.NewBool(allNodesRunningServiceAccounts)}),
		)
		return nil
	}

	// Case 2: This is an existing cluster, but actual cluster version does not support service accounts.
	if d.ES.Status.Version == "" {
		return nil
	}
	supportServiceAccounts, err := esv1.AreServiceAccountsSupported(d.ES.Status.Version)
	if err != nil {
		return err
	}
	if !supportServiceAccounts {
		d.ReconcileState.UpdateOrchestrationHints(
			d.ReconcileState.OrchestrationHints().Merge(hints.OrchestrationsHints{ServiceAccounts: optional.NewBool(false)}),
		)
		return nil
	}

	// Case 3: cluster is already running with a version that does support service account and tokens have already been created.
	// We don't however know if all nodes have been migrated and are running with the service_tokens file mounted from the configuration Secret.
	// Let's try to detect that situation by comparing the existing nodes and the ones returned by the /_security/service API.
	// Note that starting with release 2.3 the association controller does not create the service account token until Elasticsearch is annotated
	// as compatible with service accounts. This is mostly to unblock situation described in https://github.com/elastic/cloud-on-k8s/issues/5684
	if !esReachable {
		// This requires the Elasticsearch API to be available
		return nil
	}
	allPods := names(resourcesState.AllPods)
	log := ulog.FromContext(ctx)
	// Detect if some service tokens are expected
	saTokens, err := user.GetServiceAccountTokens(d.Client, d.ES)
	if err != nil {
		log.Info("Could not detect if service accounts are expected", "err", err, "namespace", d.ES.Namespace, "es_name", d.ES.Name)
		return err
	}

	allNodesRunningServiceAccounts, err := allNodesRunningServiceAccounts(ctx, saTokens, set.Make(allPods...), securityClient)
	if err != nil {
		log.Info("Could not detect if all nodes are ready for using service accounts", "err", err, "namespace", d.ES.Namespace, "es_name", d.ES.Name)
		return err
	}
	if allNodesRunningServiceAccounts != nil {
		d.ReconcileState.UpdateOrchestrationHints(
			d.ReconcileState.OrchestrationHints().Merge(hints.OrchestrationsHints{ServiceAccounts: optional.NewBool(*allNodesRunningServiceAccounts)}),
		)
	}

	return nil
}

// allNodesRunningServiceAccounts attempts to detect if all the nodes in the clusters have loaded the service_tokens file.
// It returns nil if no decision can be made, for example when there is no tokens are expected to be found.
func allNodesRunningServiceAccounts(
	ctx context.Context,
	saTokens user.ServiceAccountTokens,
	allPods set.StringSet,
	securityClient esclient.SecurityClient,
) (*bool, error) {
	if len(allPods) == 0 {
		return nil, nil
	}
	if len(saTokens) == 0 {
		// No tokens are expected: we cannot call the Elasticsearch API to detect which nodes are
		// running with the conf/service_tokens file.
		return nil, nil
	}

	// Get the namespaced service name to call the /_security/service/<namespace>/<service>/credential API
	namespacedServices := saTokens.NamespacedServices()

	// Get the nodes which have loaded tokens from the conf/service_tokens file.
	for namespacedService := range namespacedServices {
		credentials, err := securityClient.GetServiceAccountCredentials(ctx, namespacedService)
		if err != nil {
			return nil, err
		}
		diff := allPods.Diff(credentials.Nodes())
		if len(diff) == 0 {
			return ptr.To[bool](true), nil
		}
	}
	// Some nodes are running but did not show up in the security API.
	return ptr.To[bool](false), nil
}

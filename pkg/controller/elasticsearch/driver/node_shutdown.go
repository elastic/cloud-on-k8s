// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"context"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/hints"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/migration"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/shutdown"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

func newShutdownInterface(
	ctx context.Context,
	es esv1.Elasticsearch,
	client esclient.Client,
	state ESState,
	observer shutdown.Observer,
) (shutdown.Interface, error) {
	var shutdownService shutdown.Interface
	if supportsNodeShutdown(client.Version()) {
		idLookup, err := state.NodeNameToID()
		if err != nil {
			return nil, err
		}
		logger := ulog.FromContext(ctx).WithValues("namespace", es.Namespace, "es_name", es.Name)
		shutdownService = shutdown.NewNodeShutdown(client, idLookup, esclient.Remove, es.ResourceVersion, logger)
	} else {
		shutdownService = migration.NewShardMigration(es, client, client)
	}
	return shutdown.WithObserver(shutdownService, observer), nil
}

func supportsNodeShutdown(v version.Version) bool {
	return v.GTE(shutdown.MinVersion)
}

// maybeRemoveTransientSettings removes left-over transient settings if we are using node shutdown and have not removed
// the settings previously that were used in the pre-node-shutdown orchestration approach.
func (d *defaultDriver) maybeRemoveTransientSettings(ctx context.Context, c esclient.Client) error {
	if supportsNodeShutdown(c.Version()) && !d.ReconcileState.OrchestrationHints().NoTransientSettings {
		ulog.FromContext(ctx).V(1).Info("Removing transient settings", "es_name", d.ES.Name, "namespace", d.ES.Namespace)
		if err := c.RemoveTransientAllocationSettings(ctx); err != nil {
			return err
		}
		d.ReconcileState.UpdateOrchestrationHints(hints.OrchestrationsHints{NoTransientSettings: true})
	}
	return nil
}

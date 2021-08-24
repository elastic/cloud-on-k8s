// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/migration"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/shutdown"
)

func newShutdownInterface(d *defaultDriver, client esclient.Client, state ESState) (shutdown.Interface, error) {
	es := d.ES
	if d.OperatorParameters.UseNodeShutdownAPI && supportsNodeShutdown(client.Version()) {
		idLookup, err := state.NodeNameToID()
		if err != nil {
			return nil, err
		}
		logger := log.WithValues("namespace", es.Namespace, "es_name", es.Name)
		return shutdown.NewNodeShutdown(client, idLookup, esclient.Remove, es.ResourceVersion, logger), nil
	}
	return migration.NewShardMigration(es, client, client), nil
}

func supportsNodeShutdown(v version.Version) bool {
	return v.GTE(version.MustParse("7.15.0-SNAPSHOT"))
}

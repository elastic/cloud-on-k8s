// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/migration"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/shutdown"
)

func newShutdownInterface(es esv1.Elasticsearch, client esclient.Client, state ESState) (shutdown.Interface, error) {
	if supportsNodeShutdown(client.Version()) {
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
	return v.GTE(version.MustParse("7.15.2"))
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/migration"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/shutdown"
)

func newShutdownInterface(es esv1.Elasticsearch, client esclient.Client, state ESState) (shutdown.Interface, error) {
	if supportsNodeshutdown(client.Version()) {
		idLookup, err := state.NodeNameToID()
		if err != nil {
			return nil, err
		}
		return shutdown.NewNodeShutdown(client, idLookup, es.ResourceVersion), nil
	}
	return migration.NewShardMigration(es, client), nil
}

func supportsNodeshutdown(v version.Version) bool {
	return v.GTE(version.MustParse("7.14.0-SNAPSHOT"))
}

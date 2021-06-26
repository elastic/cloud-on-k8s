// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"github.com/blang/semver/v4"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/migration"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/shutdown"
)

func newShutdownInterface(es esv1.Elasticsearch, client esclient.Client, state ESState) (shutdown.Interface, error) {
	v, err := semver.Parse(es.Spec.Version)
	if err != nil {
		return nil, err
	}
	if v.GTE(semver.MustParse("7.14.0-SNAPSHOT")) {
		idLookup, err := state.NodeNameToID()
		if err != nil {
			return nil, err
		}
		return shutdown.NewNodeShutdown(client, idLookup), nil
	}
	return migration.NewShardMigration(es, client), err
}

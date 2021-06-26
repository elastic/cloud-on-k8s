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

func newShutdownInterface(es esv1.Elasticsearch, client esclient.Client) shutdown.Interface {
	// at this point we have at least once successfully parsed the version let's risk a panic
	v := semver.MustParse(es.Spec.Version)
	if v.GTE(semver.MustParse("7.14.0-SNAPSHOT")) {
		return &shutdown.NodeShutdown{}
	}
	return migration.NewShardMigration(es, client)
}

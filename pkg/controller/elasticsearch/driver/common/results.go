// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/reconcile"
)

// SharedResult contains all results from shared reconciliation that drivers need.
type SharedResult struct {
	// Meta is the metadata that should be propagated to children resources.
	Meta metadata.Metadata

	// ResourcesState is the current state of cluster resources.
	ResourcesState *reconcile.ResourcesState

	// ESClient is the Elasticsearch client for API calls.
	ESClient esclient.Client
	// ESReachable indicates whether Elasticsearch is reachable.
	ESReachable bool

	// KeystoreResources contains the keystore init container and volume.
	KeystoreResources *keystore.Resources
}

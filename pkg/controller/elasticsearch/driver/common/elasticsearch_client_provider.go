// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"context"
	"crypto/x509"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/dev"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/net"
)

// NewElasticsearchClient creates a new Elasticsearch HTTP client for this cluster using the provided user.
func NewElasticsearchClient(
	ctx context.Context,
	es *esv1.Elasticsearch,
	dialer net.Dialer,
	urlProvider esclient.URLProvider,
	user esclient.BasicAuth,
	v version.Version,
	caCerts []*x509.Certificate,
) esclient.Client {
	return esclient.NewElasticsearchClient(
		dialer,
		k8s.ExtractNamespacedName(es),
		urlProvider,
		user,
		v,
		caCerts,
		esclient.Timeout(ctx, *es),
		dev.Enabled,
	)
}

func ElasticsearchClientProvider(
	ctx context.Context,
	es *esv1.Elasticsearch,
	dialer net.Dialer,
	urlProvider esclient.URLProvider,
	user esclient.BasicAuth,
	v version.Version,
	caCerts []*x509.Certificate,
) func(existingEsClient esclient.Client) esclient.Client {
	return func(existingEsClient esclient.Client) esclient.Client {
		if existingEsClient != nil && existingEsClient.HasProperties(v, user, urlProvider, caCerts) {
			return existingEsClient
		}
		return NewElasticsearchClient(ctx, es, dialer, urlProvider, user, v, caCerts)
	}
}

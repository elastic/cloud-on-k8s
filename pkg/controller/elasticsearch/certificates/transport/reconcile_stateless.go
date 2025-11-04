// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package transport

import (
	"context"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// ReconcileStatelessTransportCertificatesSecrets reconciles the secret containing transport certificates for all nodes in the
// stateless Elasticsearch cluster.
// Secrets which are not used anymore are deleted as part of the downscale process.
func ReconcileStatelessTransportCertificatesSecrets(
	ctx context.Context,
	c k8s.Client,
	ca *certificates.CA,
	additionalCAs []byte,
	es esv1.Elasticsearch,
	rotationParams certificates.RotationParams,
	meta metadata.Metadata,
) *reconciler.Results {
	results := &reconciler.Results{}
	// We must create transport certificates for all the tiers.
	for _, tierName := range esv1.AllElasticsearchTierNames {
		cloneSetName := esv1.PodsControllerResourceName(es.Name, string(tierName))
		matchLabels := label.NewLabelSelectorForCloneSetName(es.Name, cloneSetName)
		results.WithResults(
			reconcileNodeSetTransportCertificatesSecrets(ctx, c, matchLabels, ca, additionalCAs, es, cloneSetName, rotationParams, meta),
		)
	}
	return results
}

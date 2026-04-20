// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package filesettings

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// ReconcileClusterSecrets ensures the cluster_secrets field in the file settings Secret is up-to-date.
// If the Secret does not exist yet it is created. On subsequent calls, only the cluster_secrets field
// is updated (other settings are preserved).
// This function is safe to call even when a StackConfigPolicy manages the Secret: the SCP controller
// preserves existing cluster_secrets via ApplyPolicy, so both controllers converge on the same value.
func ReconcileClusterSecrets(
	ctx context.Context,
	c k8s.Client,
	es esv1.Elasticsearch,
	clusterSecrets *commonv1.Config,
) error {
	meta := metadata.Propagate(&es, metadata.Metadata{Labels: label.NewLabels(k8s.ExtractNamespacedName(&es))})
	esNsn := k8s.ExtractNamespacedName(&es)

	// Retry conflict and already-exists errors to avoid transient lost updates when
	// SCP and ES controllers update/create the same Secret concurrently.
	return retry.OnError(retry.DefaultRetry, func(err error) bool {
		return apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err)
	}, func() error {
		fs, err := Load(ctx, c, esNsn, true, meta)
		if err != nil {
			return err
		}
		if err := fs.SetClusterSecrets(clusterSecrets); err != nil {
			return err
		}
		return fs.Save(ctx, c, &es, WithAdditiveMetadata())
	})
}

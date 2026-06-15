// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package shared

import (
	"context"

	toolsevents "k8s.io/client-go/tools/events"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	remotekeystore "github.com/elastic/cloud-on-k8s/v3/pkg/controller/remotecluster/keystore"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/stackconfigpolicy"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// BuildClusterSecrets gathers all secure settings sources (user spec.secureSettings,
// remote-cluster API keys, StackConfigPolicy) and returns them in the nested structure
// expected by Elasticsearch file-based settings cluster_secrets. It also registers
// dynamic watches so the operator reconciles when any source secret changes.
func BuildClusterSecrets(
	ctx context.Context,
	c k8s.Client,
	recorder toolsevents.EventRecorder,
	dynamicWatches watches.DynamicWatches,
	es esv1.Elasticsearch,
) (*commonv1.Config, error) {
	secretSources := keystore.WatchedSecretNames(&es)

	remoteClusterAPIKeys, err := remotekeystore.APIKeySecretSource(ctx, &es, c)
	if err != nil {
		return nil, err
	}
	secretSources = append(secretSources, remoteClusterAPIKeys...)

	policySecretSources, err := stackconfigpolicy.GetSecureSettingsSecretSourcesForResources(ctx, c, &es, "Elasticsearch")
	if err != nil {
		return nil, err
	}
	secretSources = append(secretSources, policySecretSources...)

	watcher := k8s.ExtractNamespacedName(&es)
	if err := watches.WatchUserProvidedNamespacedSecrets(
		watcher,
		dynamicWatches,
		keystore.SecureSettingsWatchName(watcher),
		secretSources,
	); err != nil {
		return nil, err
	}

	data, err := keystore.BuildSecureSettingsData(ctx, c, recorder, &es, secretSources)
	if err != nil {
		return nil, err
	}
	return &commonv1.Config{Data: data}, nil
}

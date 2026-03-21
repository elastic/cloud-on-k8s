// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	commonsettings "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// ResolveConfig computes the merged Elasticsearch configuration for all NodeSets,
// including StackConfigPolicy settings. This is called early in the reconciliation
// to determine clientAuthenticationRequired before creating the ES client.
func ResolveConfig(ctx context.Context, client k8s.Client, es esv1.Elasticsearch, ipFamily corev1.IPFamily, enterpriseFeaturesEnabled bool) (nodespec.ResolvedConfig, error) {
	ver, err := version.Parse(es.Spec.Version)
	if err != nil {
		return nodespec.ResolvedConfig{}, err
	}

	// Get policy config from StackConfigPolicy
	policyConfig, err := nodespec.GetPolicyConfig(ctx, client, es)
	if err != nil {
		return nodespec.ResolvedConfig{}, err
	}

	clientAuthenticationRequired, clientAuthenticationOverrideWarning, err := detectClientAuthenticationRequired(es, ver, ipFamily, policyConfig, enterpriseFeaturesEnabled)
	if err != nil {
		return nodespec.ResolvedConfig{}, err
	}

	// Build final configs with the determined client certificate validation state.
	nodeSetConfigs := make(map[string]settings.CanonicalConfig, len(es.Spec.NodeSets))
	for _, nodeSpec := range es.Spec.NodeSets {
		userCfg := commonv1.Config{}
		if nodeSpec.Config != nil {
			userCfg = *nodeSpec.Config
		}
		clusterHasZoneAwareness := esv1.NodeSetList(es.Spec.NodeSets).HasZoneAwareness()
		cfg, err := settings.NewMergedESConfig(
			es.Name, ver, ipFamily, es.Spec.HTTP, userCfg, policyConfig.ElasticsearchConfig,
			es.Spec.RemoteClusterServer.Enabled, es.HasRemoteClusterAPIKey(), clusterHasZoneAwareness,
			clientAuthenticationRequired,
		)
		if err != nil {
			return nodespec.ResolvedConfig{}, err
		}
		nodeSetConfigs[nodeSpec.Name] = cfg
	}

	return nodespec.ResolvedConfig{
		NodeSetConfigs:                      nodeSetConfigs,
		ClientAuthenticationRequired:        clientAuthenticationRequired,
		PolicyConfig:                        policyConfig,
		ClientAuthenticationOverrideWarning: clientAuthenticationOverrideWarning,
	}, nil
}

// detectClientAuthenticationRequired evaluates the merged configuration for all NodeSets to determine
// whether client certificate authentication is effectively required. It also detects cases where
// spec.http.tls.client.authentication is set but overridden by user or StackConfigPolicy configuration.
// This is only evaluated when enterprise features are enabled; without an enterprise license,
// client authentication is not managed by ECK (users with raw config are unaffected).
func detectClientAuthenticationRequired(
	es esv1.Elasticsearch,
	ver version.Version,
	ipFamily corev1.IPFamily,
	policyConfig nodespec.PolicyConfig,
	enterpriseFeaturesEnabled bool,
) (bool, string, error) {
	if !enterpriseFeaturesEnabled {
		return false, "", nil
	}

	overrideWarning := clientAuthenticationSpecIneffectiveWarning(
		es.Spec.HTTP.TLS.Client.Authentication,
		policyConfig.ElasticsearchConfig,
		"StackConfigPolicy",
	)

	for _, nodeSpec := range es.Spec.NodeSets {
		userCfg := commonv1.Config{}
		if nodeSpec.Config != nil {
			userCfg = *nodeSpec.Config
		}

		// Check manual override only if no policy override warning has been detected.
		if overrideWarning == "" {
			userCanonicalCfg, err := commonsettings.NewCanonicalConfigFrom(userCfg.Data)
			if err != nil {
				return false, "", err
			}
			overrideWarning = clientAuthenticationSpecIneffectiveWarning(
				es.Spec.HTTP.TLS.Client.Authentication,
				userCanonicalCfg,
				"User manual",
			)
		}
		clusterHasZoneAwareness := esv1.NodeSetList(es.Spec.NodeSets).HasZoneAwareness()

		cfg, err := settings.NewMergedESConfig(
			es.Name, ver, ipFamily, es.Spec.HTTP, userCfg, policyConfig.ElasticsearchConfig,
			es.Spec.RemoteClusterServer.Enabled, es.HasRemoteClusterAPIKey(), clusterHasZoneAwareness,
			false,
		)
		if err != nil {
			return false, "", err
		}
		if settings.HasClientAuthenticationRequired(cfg) {
			return true, overrideWarning, nil
		}
	}

	return false, overrideWarning, nil
}

// clientAuthenticationSpecIneffectiveWarning returns a non-empty warning when spec.http.tls.client.authentication
// is true but the given configuration override sets xpack.security.http.ssl.client_authentication to a non-required value.
// source identifies the origin of the override (e.g. "StackConfigPolicy", "manual") and is included in the warning.
func clientAuthenticationSpecIneffectiveWarning(specClientAuthenticationEnabled bool, overrideCfg *commonsettings.CanonicalConfig, source string) string {
	if !specClientAuthenticationEnabled {
		return ""
	}

	if val, found := configString(overrideCfg, esv1.XPackSecurityHttpSslClientAuthentication); found && val != "required" {
		return fmt.Sprintf(
			"spec.http.tls.client.authentication is ineffective due to %s configuration: %s is set to %q",
			source,
			esv1.XPackSecurityHttpSslClientAuthentication,
			val,
		)
	}

	return ""
}

func configString(cfg *commonsettings.CanonicalConfig, key string) (string, bool) {
	if cfg == nil {
		return "", false
	}
	val, err := cfg.String(key)
	if err != nil {
		return "", false
	}
	return val, true
}

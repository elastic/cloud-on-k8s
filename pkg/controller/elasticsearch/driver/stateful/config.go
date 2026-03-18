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

// ResolvedConfig holds all pre-computed configuration needed for reconciliation.
// Computing this early allows us to detect clientAuthenticationRequired before creating the ES client,
// and avoids duplicate config computation in BuildExpectedResources.
type ResolvedConfig struct {
	// NodeSetConfigs contains the merged configuration for each NodeSet.
	NodeSetConfigs []nodespec.NodeSetConfig

	// ClientAuthenticationRequired indicates whether client certificate authentication is required
	// based on the merged configuration.
	ClientAuthenticationRequired bool

	// PolicyConfig contains StackConfigPolicy settings.
	PolicyConfig nodespec.PolicyConfig

	// ClientAuthenticationOverrideWarning is set when spec.http.tls.client.authentication is enabled
	// but StackConfigPolicy overrides xpack.security.http.ssl.client_authentication to a non-required value.
	ClientAuthenticationOverrideWarning string
}

// ResolveConfig computes the merged Elasticsearch configuration for all NodeSets,
// including StackConfigPolicy settings. This is called early in the reconciliation
// to determine clientAuthenticationRequired before creating the ES client.
func ResolveConfig(ctx context.Context, client k8s.Client, es esv1.Elasticsearch, ipFamily corev1.IPFamily) (ResolvedConfig, error) {
	ver, err := version.Parse(es.Spec.Version)
	if err != nil {
		return ResolvedConfig{}, err
	}

	// Get policy config from StackConfigPolicy
	policyConfig, err := nodespec.GetPolicyConfig(ctx, client, es)
	if err != nil {
		return ResolvedConfig{}, err
	}

	clientAuthenticationOverrideWarning := clientAuthenticationSpecIneffectiveWarning(
		es.Spec.HTTP.TLS.Client.Authentication,
		policyConfig.ElasticsearchConfig,
		"StackConfigPolicy",
	)

	// First pass: detect if client certificate validation is effectively required from final merged config.
	// This pass evaluates the final merged config (defaults + user config + stack config policy)
	// without trust bundle appending.
	clientAuthenticationRequired := false
	for _, nodeSpec := range es.Spec.NodeSets {
		userCfg := commonv1.Config{}
		if nodeSpec.Config != nil {
			userCfg = *nodeSpec.Config
		}

		// Check manual override only if no policy override warning has been detected.
		if clientAuthenticationOverrideWarning == "" {
			userCanonicalCfg, err := commonsettings.NewCanonicalConfigFrom(userCfg.Data)
			if err != nil {
				return ResolvedConfig{}, err
			}
			clientAuthenticationOverrideWarning = clientAuthenticationSpecIneffectiveWarning(
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
			return ResolvedConfig{}, err
		}
		if settings.HasClientAuthenticationRequired(cfg) {
			clientAuthenticationRequired = true
			break
		}
	}

	// Second pass: build final configs with the determined client certificate validation state.
	nodeSetConfigs := make([]nodespec.NodeSetConfig, 0, len(es.Spec.NodeSets))
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
			return ResolvedConfig{}, err
		}
		nodeSetConfigs = append(nodeSetConfigs, nodespec.NodeSetConfig{
			NodeSetName: nodeSpec.Name,
			Config:      cfg,
		})
	}

	return ResolvedConfig{
		NodeSetConfigs:                      nodeSetConfigs,
		ClientAuthenticationRequired:        clientAuthenticationRequired,
		PolicyConfig:                        policyConfig,
		ClientAuthenticationOverrideWarning: clientAuthenticationOverrideWarning,
	}, nil
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

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/util/validation/field"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	common "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

const (
	objectStoreForbiddenMsg             = "objectStore is not allowed in stateful mode"
	objectStoreRequiredMsg              = "objectStore is required in stateless mode"
	tierForbiddenMsg                    = "tier is not allowed in stateful mode"
	tierResolutionErrMsg                = "cannot resolve tier for NodeSet"
	tierIndexRequiredMsg                = "at least one NodeSet with index tier is required in stateless mode"
	tierSearchRequiredMsg               = "at least one NodeSet with search tier is required in stateless mode"
	modeChangeMsg                       = "changing spec.mode is not allowed"
	statelessNodeRolesWarningMsg        = "Setting node.roles manually in stateless mode is not recommended. Node roles are automatically configured based on the tier. Only set this for debugging purposes under guidance from Elastic support."
	indexAndSearchRolesConflictMsg      = "a NodeSet cannot have both index and search roles"
	statelessRoleInStatefulMsg          = "index and search roles are only supported in stateless mode"
	statelessMinVersionMsg              = "stateless mode requires Elasticsearch version 9.4.0 or higher"
	statelessLicenseRequiredMsg         = "stateless mode requires an enterprise license"
	remoteClustersStatelessMsg          = "remote clusters are not supported in stateless mode"
	clientAuthStatelessMsg              = "client certificate authentication (mTLS) is not supported in stateless mode"
	remoteClusterServerStatelessMsg     = "remote cluster server is not supported in stateless mode"
	volumeClaimDeletePolicyStatelessMsg = "volumeClaimDeletePolicy is not applicable in stateless mode"
)

var (
	objectStorePath = field.NewPath("spec").Child("objectStore")
	modePath        = field.NewPath("spec").Child("mode")
	// TODO(#9296): update to the actual minimum version that supports stateless once confirmed.
	statelessMinVersion = version.MinFor(9, 4, 0)
)

// validModeSpecificConfig dispatches to stateful or stateless validation based on spec.mode.
func validModeSpecificConfig(es esv1.Elasticsearch) field.ErrorList {
	if es.IsStateless() {
		return validateStatelessConfig(es)
	}
	return validateStatefulConfig(es)
}

// validateStatefulConfig checks constraints specific to stateful mode.
func validateStatefulConfig(es esv1.Elasticsearch) field.ErrorList {
	var errs field.ErrorList
	if es.Spec.ObjectStore != nil {
		errs = append(errs, field.Forbidden(objectStorePath, objectStoreForbiddenMsg))
	}

	v, err := version.Parse(es.Spec.Version)
	if err != nil {
		// version validation is handled elsewhere
		return errs
	}

	for i, ns := range es.Spec.NodeSets {
		if ns.Tier != "" {
			errs = append(errs, field.Forbidden(
				field.NewPath("spec").Child("nodeSets").Index(i).Child("tier"),
				tierForbiddenMsg,
			))
		}
		errs = append(errs, validateNoStatelessRoles(ns, i, v)...)
	}
	return errs
}

// validateNoStatelessRoles rejects index and search roles in stateful mode.
func validateNoStatelessRoles(ns esv1.NodeSet, index int, v version.Version) field.ErrorList {
	if ns.Config == nil {
		return nil
	}
	cfg := esv1.ElasticsearchSettings{}
	if err := esv1.UnpackConfig(ns.Config, v, &cfg); err != nil {
		return nil
	}
	if cfg.Node != nil && (cfg.Node.HasRole(esv1.IndexRole) || cfg.Node.HasRole(esv1.SearchRole)) {
		return field.ErrorList{field.Forbidden(
			field.NewPath("spec").Child("nodeSets").Index(index).Child("config"),
			statelessRoleInStatefulMsg,
		)}
	}
	return nil
}

// validateStatelessConfig checks constraints specific to stateless mode.
func validateStatelessConfig(es esv1.Elasticsearch) field.ErrorList {
	var errs field.ErrorList

	v, err := version.Parse(es.Spec.Version)
	if err != nil {
		// version validation is handled elsewhere
		return errs
	}

	if !v.GTE(statelessMinVersion) {
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("version"), es.Spec.Version, statelessMinVersionMsg))
	}

	if es.Spec.ObjectStore == nil {
		errs = append(errs, field.Required(objectStorePath, objectStoreRequiredMsg))
	}

	if len(es.Spec.RemoteClusters) > 0 {
		errs = append(errs, field.Forbidden(field.NewPath("spec").Child("remoteClusters"), remoteClustersStatelessMsg))
	}

	if es.Spec.RemoteClusterServer.Enabled {
		errs = append(errs, field.Forbidden(field.NewPath("spec").Child("remoteClusterServer"), remoteClusterServerStatelessMsg))
	}

	if es.Spec.HTTP.TLS.Client.Authentication {
		errs = append(errs, field.Forbidden(field.NewPath("spec").Child("http", "tls", "client", "authentication"), clientAuthStatelessMsg))
	}

	if es.Spec.VolumeClaimDeletePolicy != "" {
		errs = append(errs, field.Forbidden(field.NewPath("spec").Child("volumeClaimDeletePolicy"), volumeClaimDeletePolicyStatelessMsg))
	}

	hasIndex := false
	hasSearch := false
	for i, ns := range es.Spec.NodeSets {
		tier, err := ns.ResolvedTier()
		if err != nil {
			errs = append(errs, field.Invalid(
				field.NewPath("spec").Child("nodeSets").Index(i),
				ns.Name,
				tierResolutionErrMsg+": "+err.Error(),
			))
			continue
		}
		switch tier {
		case esv1.IndexTier:
			hasIndex = hasIndex || ns.Count > 0
		case esv1.SearchTier:
			hasSearch = hasSearch || ns.Count > 0
		case esv1.MasterTier, esv1.MLTier:
			// valid tiers that don't affect the index/search requirement
		}

		errs = append(errs, validateNoConflictingRoles(ns, i, v)...)
	}

	if !hasIndex {
		errs = append(errs, field.Required(field.NewPath("spec").Child("nodeSets"), tierIndexRequiredMsg))
	}
	if !hasSearch {
		errs = append(errs, field.Required(field.NewPath("spec").Child("nodeSets"), tierSearchRequiredMsg))
	}

	return errs
}

// validateNoConflictingRoles rejects a NodeSet that has both index and search roles.
func validateNoConflictingRoles(ns esv1.NodeSet, index int, v version.Version) field.ErrorList {
	if ns.Config == nil {
		return nil
	}
	cfg := esv1.ElasticsearchSettings{}
	if err := esv1.UnpackConfig(ns.Config, v, &cfg); err != nil {
		return nil // config parsing errors are reported by hasCorrectNodeRoles
	}
	if cfg.Node != nil && cfg.Node.HasRole(esv1.IndexRole) && cfg.Node.HasRole(esv1.SearchRole) {
		return field.ErrorList{field.Forbidden(
			field.NewPath("spec").Child("nodeSets").Index(index).Child("config"),
			indexAndSearchRolesConflictMsg,
		)}
	}
	return nil
}

// effectiveMode returns the deployment mode, treating an empty value as stateful (the default).
func effectiveMode(mode esv1.ElasticsearchMode) esv1.ElasticsearchMode {
	if mode == "" {
		return esv1.ElasticsearchModeStateful
	}
	return mode
}

// noModeChange ensures the deployment mode cannot be changed between stateful and stateless.
func noModeChange(current, proposed esv1.Elasticsearch) field.ErrorList {
	if effectiveMode(current.Spec.Mode) != effectiveMode(proposed.Spec.Mode) {
		return field.ErrorList{field.Forbidden(modePath, modeChangeMsg)}
	}
	return nil
}

// validStatelessLicense checks that an enterprise license is available for stateless mode.
func validStatelessLicense(ctx context.Context, es esv1.Elasticsearch, checker license.Checker) field.ErrorList {
	if !es.IsStateless() {
		return nil
	}
	enabled, err := checker.EnterpriseFeaturesEnabled(ctx)
	if err != nil {
		ulog.FromContext(ctx).Error(err, "while checking enterprise features for stateless validation")
		return nil
	}
	if !enabled {
		return field.ErrorList{field.Forbidden(modePath, statelessLicenseRequiredMsg)}
	}
	return nil
}

// statelessNodeRolesWarning warns when node.roles is manually set in stateless mode.
func statelessNodeRolesWarning(es esv1.Elasticsearch) field.ErrorList {
	if !es.IsStateless() {
		return nil
	}
	var errs field.ErrorList
	for i, nodeSet := range es.Spec.NodeSets {
		if nodeSet.Config == nil {
			continue
		}
		config, err := common.NewCanonicalConfigFrom(nodeSet.Config.Data)
		if err != nil {
			continue
		}
		if found := config.HasKeys([]string{esv1.NodeRoles}); len(found) > 0 {
			errs = append(errs, field.Invalid(
				field.NewPath("spec").Child("nodeSets").Index(i).Child("config").Child(esv1.NodeRoles),
				esv1.NodeRoles,
				fmt.Sprintf("%s (nodeSet: %s)", statelessNodeRolesWarningMsg, nodeSet.Name),
			))
		}
	}
	return errs
}

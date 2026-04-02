// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	common "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
	essettings "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
)

const (
	zoneAwarenessAffinityDoesNotExistWarningMsg = "Zone awareness injects an Exists requirement for the topology key; DoesNotExist on the same key makes this node selector term unsatisfiable, though other OR'd terms may still allow scheduling"
	zoneAwarenessAffinityNotInWarningMsg        = "Zone awareness may conflict with required node affinity using NotIn on the topology key; this can make pods unschedulable depending on node labels"
)

var warnings = []validation{
	noUnsupportedSettings,
	validZoneAwarenessAffinityWarnings,
}

func noUnsupportedSettings(es esv1.Elasticsearch) field.ErrorList {
	var errs field.ErrorList
	for i, nodeSet := range es.Spec.NodeSets {
		if nodeSet.Config == nil {
			continue
		}
		config, err := common.NewCanonicalConfigFrom(nodeSet.Config.Data)
		if err != nil {
			errs = append(errs, field.Invalid(field.NewPath("spec").Child("nodeSets").Index(i).Child("config"), es.Spec.NodeSets[i].Config, cfgInvalidMsg))
			continue
		}
		errs = append(errs, validateSettings(config, i)...)
	}
	return errs
}

func validateSettings(config *common.CanonicalConfig, index int) field.ErrorList {
	var errs field.ErrorList
	unsupported := config.HasKeys(esv1.UnsupportedSettings)
	for _, setting := range unsupported {
		errs = append(errs, field.Forbidden(field.NewPath("spec").Child("nodeSets").Index(index).Child("config").Child(setting), unsupportedConfigErrMsg))
	}
	return errs
}

// fipsModeConsistencyWarning returns a warning message if FIPS mode is
// inconsistently configured across NodeSets. It returns an empty string when
// all NodeSets agree.
func fipsModeConsistencyWarning(es esv1.Elasticsearch) string {
	var fipsCount, total int
	for _, nodeSet := range es.Spec.NodeSets {
		userConfig := map[string]any{}
		if nodeSet.Config != nil {
			userConfig = nodeSet.Config.Data
		}
		canonicalConfig, err := common.NewCanonicalConfigFrom(userConfig)
		if err != nil {
			continue
		}
		total++
		if essettings.IsFIPSEnabled(canonicalConfig) {
			fipsCount++
		}
	}
	if fipsCount > 0 && fipsCount < total {
		return inconsistentFIPSModeWarningMsg
	}
	return ""
}

// validZoneAwarenessAffinityWarnings produces warnings for required node affinity that
// may conflict with zone-awareness topology labels:
//   - DoesNotExist on the topology key makes the containing node selector term
//     unsatisfiable because the operator injects an Exists requirement for the same key.
//     When multiple terms are OR'd, other terms may still allow scheduling, so this is a
//     warning rather than a hard error.
//   - NotIn on the topology key may make pods unschedulable depending on actual node
//     labels, but cannot be fully validated without cluster topology knowledge.
func validZoneAwarenessAffinityWarnings(es esv1.Elasticsearch) field.ErrorList {
	var warnings field.ErrorList
	for _, ref := range findNodeSelectorExpressionsForZoneAwarenessTopologyKey(es, corev1.NodeSelectorOpDoesNotExist) {
		warnings = append(warnings, field.Invalid(ref.path, ref.topologyKey, zoneAwarenessAffinityDoesNotExistWarningMsg))
	}
	for _, ref := range findNodeSelectorExpressionsForZoneAwarenessTopologyKey(es, corev1.NodeSelectorOpNotIn) {
		warnings = append(warnings, field.Invalid(ref.path, ref.topologyKey, zoneAwarenessAffinityNotInWarningMsg))
	}
	return warnings
}

func CheckForWarnings(es esv1.Elasticsearch) error {
	warnings, errors := check(es, warnings)
	if warnings != "" {
		warningError := field.ErrorList{field.Invalid(field.NewPath("spec").Child("version"), es.Spec.Version, warnings)}
		errors = append(errors, warningError...)
	}
	if len(errors) > 0 {
		return errors.ToAggregate()
	}
	return nil
}

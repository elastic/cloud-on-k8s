// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	common "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
)

const (
	zoneAwarenessAffinityDoesNotExistWarningMsg = "Zone awareness injects an Exists requirement for the topology key; DoesNotExist on the same key makes this node selector term unsatisfiable, though other OR'd terms may still allow scheduling"
	zoneAwarenessAffinityNotInWarningMsg        = "Zone awareness may conflict with required node affinity using NotIn on the topology key; this can make pods unschedulable depending on node labels"
)

var warnings = []validation{
	deprecatedStackVersionWarning,
	validZoneAwarenessAffinityWarnings,
}

// deprecatedStackVersionWarning returns a field error when the stack version is deprecated (EOL).
func deprecatedStackVersionWarning(es esv1.Elasticsearch) field.ErrorList {
	deprecationWarning, _ := commonv1.CheckDeprecatedStackVersion(es.Spec.Version)
	if deprecationWarning == "" {
		return nil
	}
	return field.ErrorList{field.Invalid(field.NewPath("spec").Child("version"), es.Spec.Version, deprecationWarning)}
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
		errs = append(errs, validateClientAuthentication(config, i)...)
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

// validateClientAuthentication reports mandatory HTTP client authentication
// (value "required") as a Forbidden field error so admission surfaces it as a
// warning, not a denial.
func validateClientAuthentication(config *common.CanonicalConfig, index int) field.ErrorList {
	forbiddenValue := "required" // we allow 'none' and 'optional' but 'required' is not supported

	var errs field.ErrorList
	value, err := config.String(esv1.XPackSecurityHttpSslClientAuthentication)
	if err != nil {
		return errs
	}
	if value == forbiddenValue {
		errs = append(errs, field.Forbidden(field.NewPath("spec").Child("nodeSets").Index(index).Child("config").Child(esv1.XPackSecurityHttpSslClientAuthentication),
			unsupportedClientAuthenticationMsg))
	}
	return errs
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

// settingsWarningsAndErrors splits noUnsupportedSettings results. Reserved-key
// violations and unsupported xpack.security.http.ssl.client_authentication
// values (Forbidden) are surfaced as non-blocking admission warnings; Invalid
// config (unparseable canonical config) must deny admission.
func settingsWarningsAndErrors(es esv1.Elasticsearch) (admission.Warnings, field.ErrorList) {
	var (
		admissionWarnings admission.Warnings
		blocking          field.ErrorList
	)
	for _, e := range noUnsupportedSettings(es) {
		switch e.Type {
		case field.ErrorTypeForbidden:
			admissionWarnings = append(admissionWarnings, fmt.Sprintf("%s: %s", e.Field, e.Detail))
		case field.ErrorTypeInvalid:
			blocking = append(blocking, e)
		default:
			blocking = append(blocking, e)
		}
	}
	return admissionWarnings, blocking
}

func CheckForWarnings(es esv1.Elasticsearch) error {
	var errs field.ErrorList
	for _, val := range warnings {
		errs = append(errs, val(es)...)
	}
	for _, e := range noUnsupportedSettings(es) {
		if e.Type == field.ErrorTypeForbidden {
			errs = append(errs, e)
		}
	}
	if len(errs) > 0 {
		return errs.ToAggregate()
	}
	return nil
}

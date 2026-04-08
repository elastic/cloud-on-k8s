// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	common "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
	commonversion "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	essettings "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
	esversion "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
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
	}
	return errs
}

func validateSettings(config *common.CanonicalConfig, index int) field.ErrorList {
	var errs field.ErrorList
	unsupported := config.HasKeys(esv1.UnsupportedSettings)
	for _, setting := range unsupported {
		errs = append(errs, field.Forbidden(field.NewPath("spec").Child("nodeSets").Index(index).Child("config").Child(setting), unsupportedConfigErrMsg))
	}
	errs = append(errs, validateClientAuthentication(config, index)...)
	return errs
}

// validateClientAuthentication reports mandatory HTTP client authentication
// (value "required") as a Forbidden field error so admission surfaces it as a
// warning, not a denial.
func validateClientAuthentication(config *common.CanonicalConfig, index int) field.ErrorList {
	const forbiddenValue = "required" // we allow 'none' and 'optional' but 'required' is not supported

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

// fipsState tracks the state of FIPS mode across NodeSets for warnings generation.
type fipsState struct {
	fipsEnabledCount                int
	parseableNodeSetCount           int
	hasUserProvidedKeystorePassword bool
	keystorePasswordOverrideUnknown bool
}

// fipsWarnings emits warnings when FIPS mode is inconsistent across NodeSets or
// when managed keystore passwords are unsupported for the requested version.
// It uses the same keystore password override detection as reconciliation
// (explicit env and envFrom), which may perform API reads against c. Missing
// envFrom refs are treated as unknown override status to keep warning paths
// non-blocking for admission.
func fipsWarnings(ctx context.Context, c k8s.Client, es esv1.Elasticsearch) (field.ErrorList, error) {
	state, err := collectFIPSState(ctx, c, es)
	if err != nil {
		return nil, err
	}

	var warnings field.ErrorList
	if state.fipsEnabledCount > 0 && state.fipsEnabledCount < state.parseableNodeSetCount {
		// Attach the warning to the nodeSets path since the inconsistency is per-NodeSet.
		warnings = append(warnings, field.Invalid(field.NewPath("spec").Child("nodeSets"), es.Spec.NodeSets, inconsistentFIPSModeWarningMsg))
	}

	ver, err := commonversion.Parse(es.Spec.Version)
	if err != nil {
		// Surface the parse failure as a warning for callers that evaluate warnings in isolation;
		// ValidateElasticsearch runs supportedVersion first, so this duplicates admission only if
		// validations are skipped.
		warnings = append(warnings, field.Invalid(field.NewPath("spec").Child("version"), es.Spec.Version, parseVersionErrMsg))
		return warnings, nil //nolint:nilerr // version parse failures are admission warnings, not returned as validation errors
	}

	if state.fipsEnabledCount > 0 &&
		!state.hasUserProvidedKeystorePassword &&
		!state.keystorePasswordOverrideUnknown &&
		ver.LT(esversion.KeystorePasswordMinVersion) {
		warnings = append(warnings, field.Invalid(field.NewPath("spec").Child("version"), es.Spec.Version, fipsManagedKeystoreUnsupportedWarningMsg))
	}
	return warnings, nil
}

// collectFIPSState gathers NodeSet-level state related to FIPS mode for warnings generation.
// It evaluates each NodeSet with the StackConfigPolicy Elasticsearch config overlaid
// so warnings match the effective FIPS setting used at reconciliation time.
// NotFound errors while resolving envFrom refs are downgraded to "override
// status unknown" for warning-only admission behavior.
func collectFIPSState(ctx context.Context, c k8s.Client, es esv1.Elasticsearch) (fipsState, error) {
	state := fipsState{}
	hasUserPwd, err := essettings.AnyNodeSetHasUserProvidedKeystorePassword(ctx, c, es.Namespace, es.Spec.NodeSets)
	state.hasUserProvidedKeystorePassword = hasUserPwd
	if err != nil {
		if apierrors.IsNotFound(err) {
			state.keystorePasswordOverrideUnknown = true
		} else {
			return fipsState{}, err
		}
	}

	policyConfig, err := essettings.GetStackConfigPolicyElasticsearchConfig(ctx, c, es)
	if err != nil {
		return fipsState{}, err
	}

	for _, nodeSet := range es.Spec.NodeSets {
		userConfig := map[string]any{}
		if nodeSet.Config != nil {
			userConfig = nodeSet.Config.Data
		}
		// Since this code path is only used for warnings, we are going to ignore any errors
		// in this whole block and continue to the next nodeSet.
		canonicalConfig, cfgErr := common.NewCanonicalConfigFrom(userConfig)
		if cfgErr != nil {
			continue
		}
		state.parseableNodeSetCount++
		if err := canonicalConfig.MergeWith(policyConfig); err != nil {
			continue
		}
		if essettings.IsFIPSEnabled(canonicalConfig) {
			state.fipsEnabledCount++
		}
	}
	return state, nil
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

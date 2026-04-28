// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"context"
	"fmt"
	"net"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	stackmon "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/stackmon/validations"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
	esversion "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
	netutil "github.com/elastic/cloud-on-k8s/v3/pkg/utils/net"
)

const (
	cfgInvalidMsg                            = "Configuration invalid"
	duplicateNodeSets                        = "NodeSet names must be unique"
	invalidNamesErrMsg                       = "Elasticsearch configuration would generate resources with invalid names"
	invalidSanIPErrMsg                       = "Invalid SAN IP address. Must be a valid IPv4 address"
	conflictingZoneAwarenessTopologyKeys     = "All zone-aware NodeSets must use the same topologyKey"
	zoneAwarenessAffinityInNoIntersectionMsg = "Required node affinity In values have no intersection with the configured zone-awareness zones; the operator injects an additional In expression for the zones, so pods will be permanently unschedulable"
	masterRequiredMsg                        = "Elasticsearch needs to have at least one master node"
	mixedRoleConfigMsg                       = "Detected a combination of node.roles and %s. Use only node.roles"
	noDowngradesMsg                          = "Downgrades are not supported"
	nodeRolesInOldVersionMsg                 = "node.roles setting is not available in this version of Elasticsearch"
	parseStoredVersionErrMsg                 = "Cannot parse current Elasticsearch version. String format must be {major}.{minor}.{patch}[-{label}]"
	parseVersionErrMsg                       = "Cannot parse Elasticsearch version. String format must be {major}.{minor}.{patch}[-{label}]"
	pvcNotMountedStatefulErrMsg              = "volume claim declared but volume not mounted in any container. Note that the Elasticsearch data volume should be named 'elasticsearch-data'"
	pvcNotMountedStatelessErrMsg             = "volume claim declared but volume not mounted in any container. Note that the stateless cache volume should be named 'elasticsearch-cache'"
	unsupportedConfigErrMsg                  = "Configuration setting is reserved for internal use. User-configured use is unsupported"
	unsupportedClientAuthenticationMsg       = "HTTP client authentication mode \"required\" is not supported when set via nodeSet configuration; use spec.http.tls.client.authentication (enterprise license) instead"
	unsupportedUpgradeMsg                    = "Unsupported version upgrade path. Check the Elasticsearch documentation for supported upgrade paths."
	unsupportedVersionMsg                    = "Unsupported version"
	notAllowedNodesLabelMsg                  = "Node label not in the exposed node labels list"
	autoscalingAnnotationUnsupportedErrMsg   = "autoscaling annotation is no longer supported"
	inconsistentFIPSModeWarningMsg           = "xpack.security.fips_mode.enabled is not consistent across all NodeSets; FIPS mode should be uniform across the cluster"
	fipsManagedKeystoreUnsupportedWarningMsg = "FIPS mode is enabled in NodeSet configuration but Elasticsearch version is below 9.4.0; the operator cannot manage the keystore password automatically."
	restartTriggerRemovedWarningMsg          = "Removing the restart-trigger annotation does not cancel an in-progress rolling restart; pods not yet restarted will still be restarted with the previous trigger value."
	restartTriggerUnchangedWarningMsg        = "Restart-trigger value unchanged; no new rolling restart will be triggered if pods already have this value."
)

type validation func(esv1.Elasticsearch) field.ErrorList

type updateValidation func(esv1.Elasticsearch, esv1.Elasticsearch) field.ErrorList

// updateValidations are the validation funcs that only apply to updates
func updateValidations(ctx context.Context, k8sClient k8s.Client, validateStorageClass bool) []updateValidation {
	return []updateValidation{
		noDowngrades,
		validUpgradePath,
		noModeChange,
		func(current esv1.Elasticsearch, proposed esv1.Elasticsearch) field.ErrorList {
			return validPVCModification(ctx, current, proposed, k8sClient, validateStorageClass)
		},
	}
}

// validations are the validation funcs that apply to creates or updates.
//
// The license check is intentionally kept here even though the webhook wrapper
// (commonwebhook.NewResourceValidator) performs the same annotation-based check.
// ValidateElasticsearch is also called directly from the reconciler
// (elasticsearch_controller.go) to guard against invalid specs when webhooks
// are not configured, and that path does not go through the wrapper.
func validations(ctx context.Context, checker license.Checker, exposedNodeLabels NodeLabels) []validation {
	return []validation{
		func(proposed esv1.Elasticsearch) field.ErrorList {
			return validNodeLabels(proposed, exposedNodeLabels)
		},
		validZoneAwarenessTopologyKeys,
		validZoneAwarenessAffinityInCompatibility,
		noUnknownFields,
		validName,
		hasCorrectNodeRoles,
		supportedVersion,
		validSanIP,
		validAutoscalingConfiguration,
		validPVCNaming,
		validPVCReservedLabels,
		validMonitoring,
		validAssociations,
		func(proposed esv1.Elasticsearch) field.ErrorList {
			if proposed.IsStateless() {
				return nil
			}
			return supportsRemoteClusterUsingAPIKey(proposed)
		},
		validModeSpecificConfig,
		func(proposed esv1.Elasticsearch) field.ErrorList {
			return validStatelessLicense(ctx, proposed, checker)
		},
		func(proposed esv1.Elasticsearch) field.ErrorList {
			return validLicenseLevel(ctx, proposed, checker)
		},
		func(proposed esv1.Elasticsearch) field.ErrorList {
			return validClientAuthentication(ctx, proposed, checker)
		},
	}
}

// validNodeLabels checks that all node labels requested via the downward-node-labels annotation
// and all zone-awareness-derived topology keys are permitted by the operator's exposed-node-labels policy.
// Zone-awareness topology keys are only validated when the policy is configured (non-empty), so that
// zone awareness works out of the box when no exposed-node-labels restriction is in place.
func validNodeLabels(proposed esv1.Elasticsearch, exposedNodeLabels NodeLabels) field.ErrorList {
	var errs field.ErrorList
	annotationValue := ""
	if proposed.Annotations != nil {
		annotationValue = proposed.Annotations[esv1.DownwardNodeLabelsAnnotation]
	}
	// firstly validate the downward-node-labels annotations
	annotationLabels := esv1.ParseDownwardNodeLabels(annotationValue)
	for nodeLabel := range annotationLabels {
		if exposedNodeLabels.IsAllowed(nodeLabel) {
			continue
		}
		errs = append(
			errs,
			field.Invalid(
				field.NewPath("metadata").Child("annotations", esv1.DownwardNodeLabelsAnnotation),
				nodeLabel,
				notAllowedNodesLabelMsg,
			),
		)
	}

	// then validate the zone-awareness-derived topology keys
	if len(exposedNodeLabels) > 0 {
		for i, nodeSet := range proposed.Spec.NodeSets {
			if nodeSet.ZoneAwareness == nil {
				continue
			}
			topologyKey := nodeSet.ZoneAwareness.TopologyKeyOrDefault()
			if exposedNodeLabels.IsAllowed(topologyKey) {
				continue
			}
			errs = append(
				errs,
				field.Invalid(
					field.NewPath("spec").Child("nodeSets").Index(i).Child("zoneAwareness", "topologyKey"),
					topologyKey,
					notAllowedNodesLabelMsg,
				),
			)
		}
	}

	return errs
}

// validZoneAwarenessTopologyKeys rejects configurations where zone-aware NodeSets
// use different topology keys. Since the operator generates a single init script for all nodeSets
// that waits for all topology keys to be present, all zone-aware NodeSets must use the same topology key.
func validZoneAwarenessTopologyKeys(es esv1.Elasticsearch) field.ErrorList {
	var errs field.ErrorList
	topologyKeys := sets.New[string]()

	for _, nodeSet := range es.Spec.NodeSets {
		if nodeSet.ZoneAwareness == nil {
			continue
		}
		topologyKeys.Insert(nodeSet.ZoneAwareness.TopologyKeyOrDefault())
	}

	if topologyKeys.Len() <= 1 {
		return nil
	}

	// If we end up here, we have multiple zone-aware NodeSets with different topology keys.
	for i, nodeSet := range es.Spec.NodeSets {
		if nodeSet.ZoneAwareness == nil {
			continue
		}
		errs = append(
			errs,
			field.Invalid(
				field.NewPath("spec").Child("nodeSets").Index(i).Child("zoneAwareness", "topologyKey"),
				nodeSet.ZoneAwareness.TopologyKeyOrDefault(),
				conflictingZoneAwarenessTopologyKeys,
			),
		)
	}
	return errs
}

// nodeSelectorExpressionRef captures a required node affinity match expression that
// targets a zone-awareness topology key. It is used by both hard validations (e.g.
// conflicting In values) and warnings (e.g. DoesNotExist, NotIn) to reference the
// exact field path, the resolved topology key, and any values from the expression.
type nodeSelectorExpressionRef struct {
	path        *field.Path
	topologyKey string
	values      []string
}

// findMatchingNodeAffinityExpressions returns required node affinity match expressions
// in the given NodeSet that match the specified topology key and operator.
func findMatchingNodeAffinityExpressions(nodeSetIdx int, nodeSet esv1.NodeSet, topologyKey string, operator corev1.NodeSelectorOperator) []nodeSelectorExpressionRef {
	affinity := nodeSet.PodTemplate.Spec.Affinity
	if affinity == nil || affinity.NodeAffinity == nil || affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		return nil
	}
	var refs []nodeSelectorExpressionRef
	for j, term := range affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
		for k, expression := range term.MatchExpressions {
			if expression.Key != topologyKey || expression.Operator != operator {
				continue
			}
			refs = append(refs, nodeSelectorExpressionRef{
				path:        field.NewPath("spec").Child("nodeSets").Index(nodeSetIdx).Child("podTemplate", "spec", "affinity", "nodeAffinity", "requiredDuringSchedulingIgnoredDuringExecution", "nodeSelectorTerms").Index(j).Child("matchExpressions").Index(k),
				topologyKey: topologyKey,
				values:      expression.Values,
			})
		}
	}
	return refs
}

// findNodeSelectorExpressionsForZoneAwarenessTopologyKey returns required node affinity
// match expressions that use the provided operator on the effective zone-awareness
// topology key for each NodeSet.
func findNodeSelectorExpressionsForZoneAwarenessTopologyKey(es esv1.Elasticsearch, operator corev1.NodeSelectorOperator) []nodeSelectorExpressionRef {
	nodeSets := esv1.NodeSetList(es.Spec.NodeSets)
	if !nodeSets.HasZoneAwareness() {
		return nil
	}
	clusterTopologyKey := nodeSets.ZoneAwarenessTopologyKey()

	refs := make([]nodeSelectorExpressionRef, 0, len(es.Spec.NodeSets))
	for i, nodeSet := range es.Spec.NodeSets {
		// Non-zone-aware NodeSets still get the cluster topology key injected at
		// runtime (as an Exists requirement), so we validate against that key.
		// Zone-aware NodeSets use their own configured key.
		topologyKey := clusterTopologyKey
		if nodeSet.ZoneAwareness != nil {
			topologyKey = nodeSet.ZoneAwareness.TopologyKeyOrDefault()
		}
		refs = append(refs, findMatchingNodeAffinityExpressions(i, nodeSet, topologyKey, operator)...)
	}
	return refs
}

// validZoneAwarenessAffinityInCompatibility rejects configurations where a zone-aware
// NodeSet specifies explicit zones and also has a required node affinity In expression
// on the same topology key whose values share no intersection with the configured zones.
// The operator injects an additional In expression for the configured zones, so the
// AND of both In expressions yields an empty set, making pods permanently unschedulable.
//
// Known gaps not covered by affinity-vs-zone-awareness validations:
//   - DoesNotExist on the topology key: handled as a warning (see warnings.go) because
//     when multiple node selector terms are OR'd, other terms may still allow scheduling.
//   - NotIn on the topology key that excludes all real zone values: cannot validate without
//     cluster topology knowledge; handled as a warning (see warnings.go).
//   - PreferredDuringScheduling affinity is soft and intentionally not checked.
func validZoneAwarenessAffinityInCompatibility(es esv1.Elasticsearch) field.ErrorList {
	var errs field.ErrorList
	for i, nodeSet := range es.Spec.NodeSets {
		if nodeSet.ZoneAwareness == nil || len(nodeSet.ZoneAwareness.Zones) == 0 {
			continue
		}
		topologyKey := nodeSet.ZoneAwareness.TopologyKeyOrDefault()
		zones := sets.New[string](nodeSet.ZoneAwareness.Zones...)

		for _, ref := range findMatchingNodeAffinityExpressions(i, nodeSet, topologyKey, corev1.NodeSelectorOpIn) {
			if zones.Intersection(sets.New[string](ref.values...)).Len() == 0 {
				errs = append(errs, field.Invalid(ref.path, ref.values, zoneAwarenessAffinityInNoIntersectionMsg))
			}
		}
	}
	return errs
}

func check(es esv1.Elasticsearch, validations []validation) field.ErrorList {
	var errs field.ErrorList
	for _, val := range validations {
		if err := val(es); err != nil {
			errs = append(errs, err...)
		}
	}
	return errs
}

// noUnknownFields checks whether the last applied config annotation contains json with unknown fields.
func noUnknownFields(es esv1.Elasticsearch) field.ErrorList {
	return commonv1.NoUnknownFields(&es, es.ObjectMeta)
}

// validName checks whether the name is valid.
func validName(es esv1.Elasticsearch) field.ErrorList {
	var errs field.ErrorList
	if err := esv1.ValidateNames(es); err != nil {
		errs = append(errs, field.Invalid(field.NewPath("metadata").Child("name"), es.Name, fmt.Sprintf("%s: %s", invalidNamesErrMsg, err)))
	}
	return errs
}

func supportedVersion(es esv1.Elasticsearch) field.ErrorList {
	ver, err := version.Parse(es.Spec.Version)
	if err != nil {
		return field.ErrorList{field.Invalid(field.NewPath("spec").Child("version"), es.Spec.Version, parseVersionErrMsg)}
	}
	if v := esversion.SupportedVersions(ver); v != nil {
		if err := v.WithinRange(ver); err == nil {
			return field.ErrorList{}
		}
	}
	return field.ErrorList{field.Invalid(field.NewPath("spec").Child("version"), es.Spec.Version, unsupportedVersionMsg)}
}

func supportsRemoteClusterUsingAPIKey(es esv1.Elasticsearch) field.ErrorList {
	ver, err := version.Parse(es.Spec.Version)
	if err != nil {
		return field.ErrorList{field.Invalid(field.NewPath("spec").Child("version"), es.Spec.Version, parseVersionErrMsg)}
	}
	var errs field.ErrorList
	if es.Spec.RemoteClusterServer.Enabled && ver.LE(esv1.RemoteClusterAPIKeysMinVersion) {
		errs = append(errs, field.Invalid(
			field.NewPath("spec").Child("remoteClusterServer"),
			es.Spec.Version,
			fmt.Sprintf(
				"minimum required version for remote cluster server is %s but desired version is %s",
				esv1.RemoteClusterAPIKeysMinVersion,
				es.Spec.Version,
			),
		))
	}
	if es.HasRemoteClusterAPIKey() && ver.LE(esv1.RemoteClusterAPIKeysMinVersion) {
		errs = append(errs, field.Invalid(
			field.NewPath("spec").Child("remoteClusters").Child("*").Key("apiKey"),
			es.Spec.Version,
			fmt.Sprintf(
				"minimum required version for remote cluster using API keys is %s but desired version is %s",
				esv1.RemoteClusterAPIKeysMinVersion,
				es.Spec.Version,
			),
		))
	}
	return errs
}

// hasCorrectNodeRoles checks whether Elasticsearch node roles are correctly configured.
// The rules are:
// There must be at least one master node (stateful only, stateless tiers handle this differently).
// node.roles are only supported on Elasticsearch 7.9.0 and above.
func hasCorrectNodeRoles(es esv1.Elasticsearch) field.ErrorList {
	v, err := version.Parse(es.Spec.Version)
	if err != nil {
		return field.ErrorList{field.Invalid(field.NewPath("spec").Child("version"), es.Spec.Version, parseVersionErrMsg)}
	}

	seenMaster := false

	var errs field.ErrorList

	confField := func(index int) *field.Path {
		return field.NewPath("spec").Child("nodeSets").Index(index).Child("config")
	}

	for i, ns := range es.Spec.NodeSets {
		cfg := esv1.ElasticsearchSettings{}
		if err := esv1.UnpackConfig(ns.Config, v, &cfg); err != nil {
			errs = append(errs, field.Invalid(confField(i), ns.Config, cfgInvalidMsg))

			continue
		}

		// check that node.roles is not used with an older Elasticsearch version
		if cfg.Node != nil && cfg.Node.Roles != nil && !v.GTE(version.From(7, 9, 0)) {
			errs = append(errs, field.Invalid(confField(i), ns.Config, nodeRolesInOldVersionMsg))

			continue
		}

		// check that node.roles and node attributes are not mixed
		nodeRoleAttrs := getNodeRoleAttrs(cfg)
		if cfg.Node != nil && len(cfg.Node.Roles) > 0 && len(nodeRoleAttrs) > 0 {
			errs = append(errs, field.Forbidden(confField(i), fmt.Sprintf(mixedRoleConfigMsg, strings.Join(nodeRoleAttrs, ","))))
		}

		// Check if this nodeSet has the master role.
		seenMaster = seenMaster || (cfg.Node.IsConfiguredWithRole(esv1.MasterRole) && !cfg.Node.IsConfiguredWithRole(esv1.VotingOnlyRole) && ns.Count > 0)
	}

	if !seenMaster && !es.IsStateless() {
		errs = append(errs, field.Required(field.NewPath("spec").Child("nodeSets"), masterRequiredMsg))
	}

	return errs
}

func getNodeRoleAttrs(cfg esv1.ElasticsearchSettings) []string {
	var nodeRoleAttrs []string

	//nolint:nestif
	if cfg.Node != nil {
		if cfg.Node.Data != nil {
			nodeRoleAttrs = append(nodeRoleAttrs, esv1.NodeData)
		}

		if cfg.Node.Ingest != nil {
			nodeRoleAttrs = append(nodeRoleAttrs, esv1.NodeIngest)
		}

		if cfg.Node.Master != nil {
			nodeRoleAttrs = append(nodeRoleAttrs, esv1.NodeMaster)
		}

		if cfg.Node.ML != nil {
			nodeRoleAttrs = append(nodeRoleAttrs, esv1.NodeML)
		}

		if cfg.Node.RemoteClusterClient != nil {
			nodeRoleAttrs = append(nodeRoleAttrs, esv1.NodeRemoteClusterClient)
		}

		if cfg.Node.Transform != nil {
			nodeRoleAttrs = append(nodeRoleAttrs, esv1.NodeTransform)
		}

		if cfg.Node.VotingOnly != nil {
			nodeRoleAttrs = append(nodeRoleAttrs, esv1.NodeVotingOnly)
		}
	}

	return nodeRoleAttrs
}

func validSanIP(es esv1.Elasticsearch) field.ErrorList {
	var errs field.ErrorList
	selfSignedCerts := es.Spec.HTTP.TLS.SelfSignedCertificate
	if selfSignedCerts != nil {
		for _, san := range selfSignedCerts.SubjectAlternativeNames {
			if san.IP != "" {
				ip := netutil.IPToRFCForm(net.ParseIP(san.IP))
				if ip == nil {
					errs = append(errs, field.Invalid(field.NewPath("spec").Child("http", "tls", "selfSignedCertificate", "subjectAlternativeNames"), san.IP, invalidSanIPErrMsg))
				}
			}
		}
	}
	return errs
}

func checkNodeSetNameUniqueness(es esv1.Elasticsearch) field.ErrorList {
	var errs field.ErrorList
	nodeSets := es.Spec.NodeSets
	names := make(map[string]struct{})
	duplicates := make(map[string]struct{})
	for _, nodeSet := range nodeSets {
		if _, found := names[nodeSet.Name]; found {
			duplicates[nodeSet.Name] = struct{}{}
		}
		names[nodeSet.Name] = struct{}{}
	}
	for _, dupe := range duplicates {
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("nodeSets"), dupe, duplicateNodeSets))
	}
	return errs
}

func noDowngrades(current, proposed esv1.Elasticsearch) field.ErrorList {
	var errs field.ErrorList

	// allow disabling version validation
	if proposed.IsConfiguredToAllowDowngrades() {
		return errs
	}

	currentVer, err := version.Parse(current.Spec.Version)
	if err != nil {
		// this should not happen, since this is the already persisted version
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("version"), current.Spec.Version, parseStoredVersionErrMsg))
	}
	proposedVer, err := version.Parse(proposed.Spec.Version)
	if err != nil {
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("version"), proposed.Spec.Version, parseVersionErrMsg))
	}
	if len(errs) != 0 {
		return errs
	}
	if !proposedVer.GTE(currentVer) {
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("version"), proposed.Spec.Version, noDowngradesMsg))
	}
	return errs
}

func validUpgradePath(current, proposed esv1.Elasticsearch) field.ErrorList {
	var errs field.ErrorList
	currentVer, ferr := currentVersion(current)
	if ferr != nil {
		errs = append(errs, ferr)
	}

	proposedVer, err := version.Parse(proposed.Spec.Version)
	if err != nil {
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("version"), proposed.Spec.Version, parseVersionErrMsg))
	}
	if len(errs) != 0 {
		return errs
	}

	supportedVersions := esversion.SupportedVersions(proposedVer)
	if supportedVersions == nil {
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("version"), proposed.Spec.Version, unsupportedVersionMsg))
		return errs
	}

	err = supportedVersions.WithinRange(currentVer)
	if err != nil {
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("version"), proposed.Spec.Version, unsupportedUpgradeMsg))
	}
	return errs
}

func currentVersion(current esv1.Elasticsearch) (version.Version, *field.Error) {
	// we do not have a version in the status let's use the version in the current spec instead which will not reflect
	// actually running Pods but which is still better than no validation.
	if current.Status.Version == "" {
		currentVer, err := version.Parse(current.Spec.Version)
		if err != nil {
			// this should not happen, since this is the already persisted version
			return version.Version{}, field.Invalid(field.NewPath("spec").Child("version"), current.Spec.Version, parseStoredVersionErrMsg)
		}
		return currentVer, nil
	}
	// if available use the status version which reflects the lowest version currently running in the cluster
	currentVer, err := version.Parse(current.Status.Version)
	if err != nil {
		// this should not happen, since this is the version from the spec copied to the status by the operator
		return version.Version{}, field.Invalid(field.NewPath("status").Child("version"), current.Status.Version, parseStoredVersionErrMsg)
	}
	return currentVer, nil
}

func validMonitoring(es esv1.Elasticsearch) field.ErrorList {
	return stackmon.Validate(&es, es.Spec.Version, stackmon.MinStackVersion)
}

func validAssociations(es esv1.Elasticsearch) field.ErrorList {
	monitoringPath := field.NewPath("spec").Child("monitoring")
	err1 := commonv1.CheckAssociationRefs(monitoringPath.Child("metrics"), es.GetMonitoringMetricsRefs()...)
	err2 := commonv1.CheckAssociationRefs(monitoringPath.Child("logs"), es.GetMonitoringLogsRefs()...)
	return append(err1, err2...)
}

func validLicenseLevel(ctx context.Context, es esv1.Elasticsearch, checker license.Checker) field.ErrorList {
	var errs field.ErrorList
	ok, err := license.HasRequestedLicenseLevel(ctx, es.Annotations, checker)
	if err != nil {
		ulog.FromContext(ctx).Error(err, "while checking license level during validation")
		return nil
	}
	if !ok {
		errs = append(errs, field.Invalid(field.NewPath("metadata").Child("annotations").Child(license.Annotation), "enterprise", "Enterprise license required but ECK operator is running on a Basic license"))
	}
	return errs
}

func validateRestartAllocationDelayWarnings(es esv1.Elasticsearch) string {
	_, err := esv1.GetRestartAllocationDelayAnnotation(es.Annotations)
	if err != nil {
		return fmt.Sprintf("restart-allocation-delay annotation will be ignored due to error: %s", err.Error())
	}

	return ""
}

func validateRestartTriggerWarnings(ctx context.Context, k8sClient k8s.Client, oldCR, newCR esv1.Elasticsearch) string {
	oldRestartTrigger := oldCR.Annotations[esv1.RestartTriggerAnnotation]
	newRestartTrigger := newCR.Annotations[esv1.RestartTriggerAnnotation]

	// No warning check is needed when:
	//   1. No change: old and new values are the same.
	//   2. Transition: both are set but different (user is explicitly changing the trigger);
	//      a new rolling restart will be triggered as expected.
	if oldRestartTrigger == newRestartTrigger || (newRestartTrigger != "" && oldRestartTrigger != "") {
		return ""
	}

	log := ulog.FromContext(ctx)

	pods, err := sset.GetActualPodsForCluster(k8sClient, oldCR)
	if err != nil {
		log.Error(err, "while fetching pods for restart trigger validation")
		return ""
	}

	// check if restart is in already progress
	restartInProgress := false
	for _, p := range pods {
		if p.Annotations[esv1.RestartTriggerAnnotation] != oldRestartTrigger {
			restartInProgress = true
		}
	}

	// Warning 1: user has removed the restart annotation (while rolling restart in-progress);
	// Restart will not be stopped.
	if newRestartTrigger == "" && restartInProgress {
		return restartTriggerRemovedWarningMsg
	}

	// Warning 2: user is setting the annotation (that was previously unset) to a value that pods already
	// have from previous restart trigger that was removed.
	// No new rolling restart will be triggered.
	if oldRestartTrigger == "" && newRestartTrigger == sset.GetActualPodsRestartTriggerAnnotationFromPods(pods) {
		return restartTriggerUnchangedWarningMsg
	}

	return ""
}

// validClientAuthentication checks that client certificate authentication is only enabled with an enterprise license.
// This intentionally only gates spec.http.tls.client.authentication (the ECK-managed path) and does not check
// the raw config path (xpack.security.http.ssl.client_authentication) for two reasons:
// 1. Gating the raw config path would be a breaking change for users who manually configured client auth before this feature.
// 2. StackConfigPolicy-driven client auth uses the raw config path and is already gated by its own enterprise license check.
func validClientAuthentication(ctx context.Context, es esv1.Elasticsearch, checker license.Checker) field.ErrorList {
	if !es.Spec.HTTP.TLS.Client.Authentication {
		return nil
	}
	if !es.Spec.HTTP.TLS.Enabled() {
		return field.ErrorList{
			field.Invalid(
				field.NewPath("spec").Child("http", "tls", "client", "authentication"),
				true,
				"client certificate authentication requires TLS to be enabled",
			),
		}
	}
	enabled, err := checker.EnterpriseFeaturesEnabled(ctx)
	if err != nil {
		ulog.FromContext(ctx).Error(err, "while checking enterprise features during client authentication validation")
		return nil
	}
	if !enabled {
		return field.ErrorList{
			field.Forbidden(
				field.NewPath("spec").Child("http", "tls", "client", "authentication"),
				"client certificate authentication requires an enterprise license",
			),
		}
	}
	return nil
}

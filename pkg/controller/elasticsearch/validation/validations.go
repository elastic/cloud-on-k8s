// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"fmt"
	"net"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation/field"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	stackmon "github.com/elastic/cloud-on-k8s/pkg/controller/common/stackmon/validations"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	esversion "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/version"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/pkg/utils/log"
	netutil "github.com/elastic/cloud-on-k8s/pkg/utils/net"
)

var log = ulog.Log.WithName("es-validation")

const (
	autoscalingVersionMsg    = "autoscaling is not available in this version of Elasticsearch"
	cfgInvalidMsg            = "Configuration invalid"
	duplicateNodeSets        = "NodeSet names must be unique"
	invalidNamesErrMsg       = "Elasticsearch configuration would generate resources with invalid names"
	invalidSanIPErrMsg       = "Invalid SAN IP address. Must be a valid IPv4 address"
	masterRequiredMsg        = "Elasticsearch needs to have at least one master node"
	mixedRoleConfigMsg       = "Detected a combination of node.roles and %s. Use only node.roles"
	noDowngradesMsg          = "Downgrades are not supported"
	nodeRolesInOldVersionMsg = "node.roles setting is not available in this version of Elasticsearch"
	parseStoredVersionErrMsg = "Cannot parse current Elasticsearch version. String format must be {major}.{minor}.{patch}[-{label}]"
	parseVersionErrMsg       = "Cannot parse Elasticsearch version. String format must be {major}.{minor}.{patch}[-{label}]"
	pvcImmutableErrMsg       = "volume claim templates can only have their storage requests increased, if the storage class allows volume expansion. Any other change is forbidden"
	pvcNotMountedErrMsg      = "volume claim declared but volume not mounted in any container. Note that the Elasticsearch data volume should be named 'elasticsearch-data'"
	unsupportedConfigErrMsg  = "Configuration setting is reserved for internal use. User-configured use is unsupported"
	unsupportedUpgradeMsg    = "Unsupported version upgrade path. Check the Elasticsearch documentation for supported upgrade paths."
	unsupportedVersionMsg    = "Unsupported version"
	notAllowedNodesLabelMsg  = "Node label not in the exposed node labels list"
)

type validation func(esv1.Elasticsearch) field.ErrorList

type updateValidation func(esv1.Elasticsearch, esv1.Elasticsearch) field.ErrorList

// updateValidations are the validation funcs that only apply to updates
func updateValidations(k8sClient k8s.Client, validateStorageClass bool) []updateValidation {
	return []updateValidation{
		noDowngrades,
		validUpgradePath,
		func(current esv1.Elasticsearch, proposed esv1.Elasticsearch) field.ErrorList {
			return validPVCModification(current, proposed, k8sClient, validateStorageClass)
		},
	}
}

// validations are the validation funcs that apply to creates or updates
func validations(exposedNodeLabels NodeLabels) []validation {
	return []validation{
		func(proposed esv1.Elasticsearch) field.ErrorList {
			return validNodeLabels(proposed, exposedNodeLabels)
		},
		noUnknownFields,
		validName,
		hasCorrectNodeRoles,
		supportedVersion,
		validSanIP,
		validAutoscalingConfiguration,
		validPVCNaming,
		validMonitoring,
	}
}

func validNodeLabels(proposed esv1.Elasticsearch, exposedNodeLabels NodeLabels) field.ErrorList {
	var errs field.ErrorList
	for _, nodeLabel := range proposed.DownwardNodeLabels() {
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

// hasCorrectNodeRoles checks whether Elasticsearch node roles are correctly configured.
// The rules are:
// There must be at least one master node.
// node.roles are only supported on Elasticsearch 7.9.0 and above
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

		// Check if this nodeSet has the master role. If autoscaling is enabled the count value in the NodeSet might not be initially set.
		seenMaster = seenMaster || (cfg.Node.IsConfiguredWithRole(esv1.MasterRole) && !cfg.Node.IsConfiguredWithRole(esv1.VotingOnlyRole) && ns.Count > 0) || es.IsAutoscalingDefined()
	}

	if !seenMaster {
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
	return stackmon.Validate(&es, es.Spec.Version)
}

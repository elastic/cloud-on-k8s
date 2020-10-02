// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1

import (
	"fmt"
	"net"
	"strings"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	esversion "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/version"
	netutil "github.com/elastic/cloud-on-k8s/pkg/utils/net"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

const (
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
	pvcImmutableMsg          = "Volume claim templates can only have their storage requests increased, if the storage class allows volume expansion. Any other change is forbidden."
	unsupportedConfigErrMsg  = "Configuration setting is reserved for internal use. User-configured use is unsupported"
	unsupportedUpgradeMsg    = "Unsupported version upgrade path. Check the Elasticsearch documentation for supported upgrade paths."
	unsupportedVersionMsg    = "Unsupported version"
)

type validation func(*Elasticsearch) field.ErrorList

// validations are the validation funcs that apply to creates or updates
var validations = []validation{
	noUnknownFields,
	validName,
	hasCorrectNodeRoles,
	supportedVersion,
	validSanIP,
}

type updateValidation func(*Elasticsearch, *Elasticsearch) field.ErrorList

// updateValidations are the validation funcs that only apply to updates
var updateValidations = []updateValidation{
	noDowngrades,
	validUpgradePath,
	pvcModification,
}

func (es *Elasticsearch) check(validations []validation) field.ErrorList {
	var errs field.ErrorList
	for _, val := range validations {
		if err := val(es); err != nil {
			errs = append(errs, err...)
		}
	}
	return errs
}

// noUnknownFields checks whether the last applied config annotation contains json with unknown fields.
func noUnknownFields(es *Elasticsearch) field.ErrorList {
	return commonv1.NoUnknownFields(es, es.ObjectMeta)
}

// validName checks whether the name is valid.
func validName(es *Elasticsearch) field.ErrorList {
	var errs field.ErrorList
	if err := validateNames(es); err != nil {
		errs = append(errs, field.Invalid(field.NewPath("metadata").Child("name"), es.Name, fmt.Sprintf("%s: %s", invalidNamesErrMsg, err)))
	}
	return errs
}

func supportedVersion(es *Elasticsearch) field.ErrorList {
	ver, err := version.Parse(es.Spec.Version)
	if err != nil {
		return field.ErrorList{field.Invalid(field.NewPath("spec").Child("version"), es.Spec.Version, parseVersionErrMsg)}
	}
	if v := esversion.SupportedVersions(*ver); v != nil {
		if err := v.Supports(*ver); err == nil {
			return field.ErrorList{}
		}
	}
	return field.ErrorList{field.Invalid(field.NewPath("spec").Child("version"), es.Spec.Version, unsupportedVersionMsg)}
}

// hasCorrectNodeRoles checks whether Elasticsearch node roles are correctly configured.
// The rules are:
// There must be at least one master node.
// node.roles are only supported on Elasticsearch 7.9.0 and above
func hasCorrectNodeRoles(es *Elasticsearch) field.ErrorList {
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
		cfg := &ElasticsearchSettings{}
		if err := UnpackConfig(ns.Config, *v, cfg); err != nil {
			errs = append(errs, field.Invalid(confField(i), ns.Config, cfgInvalidMsg))

			continue
		}

		// check that node.roles is not used with an older Elasticsearch version
		if cfg.Node != nil && cfg.Node.Roles != nil && !v.IsSameOrAfter(version.From(7, 9, 0)) {
			errs = append(errs, field.Invalid(confField(i), ns.Config, nodeRolesInOldVersionMsg))

			continue
		}

		// check that node.roles and node attributes are not mixed
		nodeRoleAttrs := getNodeRoleAttrs(cfg)
		if cfg.Node != nil && len(cfg.Node.Roles) > 0 && len(nodeRoleAttrs) > 0 {
			errs = append(errs, field.Forbidden(confField(i), fmt.Sprintf(mixedRoleConfigMsg, strings.Join(nodeRoleAttrs, ","))))
		}

		// check if this nodeSet has the master role
		seenMaster = seenMaster || (cfg.Node.HasMasterRole() && !cfg.Node.HasVotingOnlyRole() && ns.Count > 0)
	}

	if !seenMaster {
		errs = append(errs, field.Required(field.NewPath("spec").Child("nodeSets"), masterRequiredMsg))
	}

	return errs
}

func getNodeRoleAttrs(cfg *ElasticsearchSettings) []string {
	var nodeRoleAttrs []string

	if cfg.Node != nil {
		if cfg.Node.Data != nil {
			nodeRoleAttrs = append(nodeRoleAttrs, NodeData)
		}

		if cfg.Node.Ingest != nil {
			nodeRoleAttrs = append(nodeRoleAttrs, NodeIngest)
		}

		if cfg.Node.Master != nil {
			nodeRoleAttrs = append(nodeRoleAttrs, NodeMaster)
		}

		if cfg.Node.ML != nil {
			nodeRoleAttrs = append(nodeRoleAttrs, NodeML)
		}

		if cfg.Node.Transform != nil {
			nodeRoleAttrs = append(nodeRoleAttrs, NodeTransform)
		}

		if cfg.Node.VotingOnly != nil {
			nodeRoleAttrs = append(nodeRoleAttrs, NodeVotingOnly)
		}
	}

	return nodeRoleAttrs
}

func validSanIP(es *Elasticsearch) field.ErrorList {
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

func checkNodeSetNameUniqueness(es *Elasticsearch) field.ErrorList {
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

// pvcModification ensures the only part of volume claim templates that can be changed is storage requests.
// VolumeClaimTemplates are immutable in StatefulSets, but we still manage to deal with volume expansion.
func pvcModification(current, proposed *Elasticsearch) field.ErrorList {
	var errs field.ErrorList
	if current == nil || proposed == nil {
		return errs
	}
	for i, proposedNodeSet := range proposed.Spec.NodeSets {
		currNodeSet := getNode(proposedNodeSet.Name, current)
		if currNodeSet == nil {
			// this is a new sset, so there is nothing to check
			continue
		}

		// Check that no modification was made to the volumeClaimTemplates, except on storage requests.
		// Checking semantic equality here allows providing PVC storage size with different units (eg. 1Ti vs. 1024Gi).
		// We could conceptually also disallow storage decrease here (unsupported), but it would make it hard for users
		// to revert to the previous size if tried increasing but their storage class doesn't support expansion.
		currentClaims := currNodeSet.VolumeClaimTemplates
		proposedClaims := proposedNodeSet.VolumeClaimTemplates
		if !apiequality.Semantic.DeepEqual(claimsWithoutStorageReq(currentClaims), claimsWithoutStorageReq(proposedClaims)) {
			errs = append(errs, field.Invalid(field.NewPath("spec").Child("nodeSet").Index(i).Child("volumeClaimTemplates"), proposedClaims, pvcImmutableMsg))
		}
	}
	return errs
}

// claimsWithoutStorageReq returns a copy of the given claims, with all storage requests set to the empty quantity.
func claimsWithoutStorageReq(claims []corev1.PersistentVolumeClaim) []corev1.PersistentVolumeClaim {
	result := make([]corev1.PersistentVolumeClaim, 0, len(claims))
	for _, claim := range claims {
		patchedClaim := *claim.DeepCopy()
		patchedClaim.Spec.Resources.Requests[corev1.ResourceStorage] = resource.Quantity{}
		result = append(result, patchedClaim)
	}
	return result
}

func noDowngrades(current, proposed *Elasticsearch) field.ErrorList {
	var errs field.ErrorList
	if current == nil || proposed == nil {
		return errs
	}
	currentVer, err := version.Parse(current.Spec.Version)
	if err != nil {
		// this should not happen, since this is the already persisted version
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("version"), current.Spec.Version, parseStoredVersionErrMsg))
	}
	currVer, err := version.Parse(proposed.Spec.Version)
	if err != nil {
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("version"), proposed.Spec.Version, parseVersionErrMsg))
	}
	if len(errs) != 0 {
		return errs
	}
	if !currVer.IsSameOrAfter(*currentVer) {
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("version"), proposed.Spec.Version, noDowngradesMsg))
	}
	return errs
}

func validUpgradePath(current, proposed *Elasticsearch) field.ErrorList {
	var errs field.ErrorList
	if current == nil || proposed == nil {
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

	v := esversion.SupportedVersions(*proposedVer)
	if v == nil {
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("version"), proposed.Spec.Version, unsupportedVersionMsg))
		return errs
	}

	err = v.Supports(*currentVer)
	if err != nil {
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("version"), proposed.Spec.Version, unsupportedUpgradeMsg))
	}
	return errs
}

func getNode(name string, es *Elasticsearch) *NodeSet {
	for i := range es.Spec.NodeSets {
		if es.Spec.NodeSets[i].Name == name {
			return &es.Spec.NodeSets[i]
		}
	}
	return nil
}

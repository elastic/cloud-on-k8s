// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package validation

import (
	"errors"
	"fmt"
	"net"
	"strings"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	esversion "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/version"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	netutil "github.com/elastic/cloud-on-k8s/pkg/utils/net"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("es-validation")

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
	pvcImmutableErrMsg       = "volume claim templates can only have their storage requests increased, if the storage class allows volume expansion. Any other change is forbidden"
	unsupportedConfigErrMsg  = "Configuration setting is reserved for internal use. User-configured use is unsupported"
	unsupportedUpgradeMsg    = "Unsupported version upgrade path. Check the Elasticsearch documentation for supported upgrade paths."
	unsupportedVersionMsg    = "Unsupported version"
)

type validation func(esv1.Elasticsearch) field.ErrorList

// validations are the validation funcs that apply to creates or updates
var validations = []validation{
	noUnknownFields,
	validName,
	hasCorrectNodeRoles,
	supportedVersion,
	validSanIP,
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
		if err := esv1.UnpackConfig(ns.Config, *v, &cfg); err != nil {
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

func getNodeRoleAttrs(cfg esv1.ElasticsearchSettings) []string {
	var nodeRoleAttrs []string

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

func validUpgradePath(current, proposed esv1.Elasticsearch) field.ErrorList {
	var errs field.ErrorList
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
func ValidateClaimsUpdate(
func ValidateClaimsStorageUpdate(
	k8sClient k8s.Client,
	initial []corev1.PersistentVolumeClaim,
	updated []corev1.PersistentVolumeClaim,
	validateStorageClass bool,
) error {
	for _, updatedClaim := range updated {
		initialClaim := claimMatchingName(initial, updatedClaim.Name)
		if initialClaim == nil {
			// existing claim does not exist in updated
			return errors.New(pvcImmutableErrMsg)
		}

		cmp := k8s.CompareStorageRequests(initialClaim.Spec.Resources, updatedClaim.Spec.Resources)
		switch {
		case cmp.Increase:
			// storage increase requested: ensure the storage class allows volume expansion
			if err := EnsureClaimSupportsExpansion(k8sClient, updatedClaim, validateStorageClass); err != nil {
				return err
			}
		case cmp.Decrease:
			// storage decrease is not supported
			return fmt.Errorf("decreasing storage size is not supported: an attempt was made to decrease storage size for claim %s", updatedClaim.Name)
		}
	}
	return nil
}

func claimMatchingName(claims []corev1.PersistentVolumeClaim, name string) *corev1.PersistentVolumeClaim {
	for i, claim := range claims {
		if claim.Name == name {
			return &claims[i]
		}
	}
	return nil
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

// EnsureClaimSupportsExpansion inspects whether the storage class referenced by the claim
// allows volume expansion, and returns an error if it doesn't.
func EnsureClaimSupportsExpansion(k8sClient k8s.Client, claim corev1.PersistentVolumeClaim, validateStorageClass bool) error {
	if !validateStorageClass {
		log.V(1).Info("Skipping storage class validation")
		return nil
	}
	sc, err := getStorageClass(k8sClient, claim)
	if err != nil {
		return err
	}
	if !allowsVolumeExpansion(sc) {
		return fmt.Errorf("claim %s (storage class %s) does not support volume expansion", claim.Name, sc.Name)
	}
	return nil
}

// getStorageClass returns the storage class specified by the given claim,
// or the default storage class if the claim does not specify any.
func getStorageClass(k8sClient k8s.Client, claim corev1.PersistentVolumeClaim) (storagev1.StorageClass, error) {
	if claim.Spec.StorageClassName == nil || *claim.Spec.StorageClassName == "" {
		return getDefaultStorageClass(k8sClient)
	}
	var sc storagev1.StorageClass
	if err := k8sClient.Get(types.NamespacedName{Name: *claim.Spec.StorageClassName}, &sc); err != nil {
		return storagev1.StorageClass{}, fmt.Errorf("cannot retrieve storage class: %w", err)
	}
	return sc, nil
}

// getDefaultStorageClass returns the default storage class in the current k8s cluster,
// or an error if there is none.
func getDefaultStorageClass(k8sClient k8s.Client) (storagev1.StorageClass, error) {
	var scs storagev1.StorageClassList
	if err := k8sClient.List(&scs); err != nil {
		return storagev1.StorageClass{}, err
	}
	for _, sc := range scs.Items {
		if isDefaultStorageClass(sc) {
			return sc, nil
		}
	}
	return storagev1.StorageClass{}, errors.New("no default storage class found")
}

// isDefaultStorageClass inspects the given storage class and returns true if it is annotated as the default one.
func isDefaultStorageClass(sc storagev1.StorageClass) bool {
	if len(sc.Annotations) == 0 {
		return false
	}
	if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" ||
		sc.Annotations["storageclass.beta.kubernetes.io/is-default-class"] == "true" {
		return true
	}
	return false
}

// allowsVolumeExpansion returns true if the given storage class allows volume expansion.
func allowsVolumeExpansion(sc storagev1.StorageClass) bool {
	return sc.AllowVolumeExpansion != nil && *sc.AllowVolumeExpansion
}

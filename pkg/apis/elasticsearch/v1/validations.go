// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1

import (
	"fmt"
	"net"
	"reflect"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	esversion "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/version"
	netutil "github.com/elastic/cloud-on-k8s/pkg/utils/net"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

const (
	cfgInvalidMsg            = "Configuration invalid"
	masterRequiredMsg        = "Elasticsearch needs to have at least one master node"
	parseVersionErrMsg       = "Cannot parse Elasticsearch version"
	parseStoredVersionErrMsg = "Cannot parse current Elasticsearch version"
	invalidSanIPErrMsg       = "Invalid SAN IP address"
	pvcImmutableMsg          = "Volume claim templates cannot be modified"
	invalidNamesErrMsg       = "Elasticsearch configuration would generate resources with invalid names"
	unsupportedVersionErrMsg = "Unsupported version"
	unsupportedConfigErrMsg  = "Configuration setting is reserved for internal use. User-configured use is unsupported"
	duplicateNodeSets        = "NodeSet names must be unique"
	noDowngradesMsg          = "Downgrades are not supported"
	unsupportedVersionMsg    = "Unsupported version"
	unsupportedUpgradeMsg    = "Unsupported version upgrade path"
)

type validation func(*Elasticsearch) field.ErrorList

// validations are the validation funcs that apply to creates or updates
var validations = []validation{
	noUnknownFields,
	validName,
	hasMaster,
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

func (r *Elasticsearch) check(validations []validation) field.ErrorList {
	var errs field.ErrorList
	for _, val := range validations {
		if err := val(r); err != nil {
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
	return field.ErrorList{field.Invalid(field.NewPath("spec").Child("version"), es.Spec.Version, unsupportedVersionErrMsg)}
}

// hasMaster checks if the given Elasticsearch cluster has at least one master node.
func hasMaster(es *Elasticsearch) field.ErrorList {
	var errs field.ErrorList
	var hasMaster bool
	for i, t := range es.Spec.NodeSets {
		cfg, err := UnpackConfig(t.Config)
		if err != nil {
			errs = append(errs, field.Invalid(field.NewPath("spec").Child("nodeSets").Index(i), t.Config, cfgInvalidMsg))
		}
		hasMaster = hasMaster || (cfg.Node.Master && t.Count > 0)
	}
	if !hasMaster {
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("nodeSets"), es.Spec.NodeSets, masterRequiredMsg))
	}
	return errs
}

func validSanIP(es *Elasticsearch) field.ErrorList {
	var errs field.ErrorList
	selfSignedCerts := es.Spec.HTTP.TLS.SelfSignedCertificate
	if selfSignedCerts != nil {
		for _, san := range selfSignedCerts.SubjectAlternativeNames {
			if san.IP != "" {
				ip := netutil.MaybeIPTo4(net.ParseIP(san.IP))
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

// pvcModification ensures no PVCs are changed, as volume claim templates are immutable in stateful sets
func pvcModification(current, proposed *Elasticsearch) field.ErrorList {
	var errs field.ErrorList
	if current == nil || proposed == nil {
		return errs
	}
	for i, node := range proposed.Spec.NodeSets {
		currNode := getNode(node.Name, current)
		if currNode == nil {
			// this is a new sset, so there is nothing to check
			continue
		}

		// ssets do not allow modifications to fields other than 'replicas', 'template', and 'updateStrategy'
		// reflection isn't ideal, but okay here since the ES object does not have the status of the claims
		if !reflect.DeepEqual(node.VolumeClaimTemplates, currNode.VolumeClaimTemplates) {
			errs = append(errs, field.Invalid(field.NewPath("spec").Child("nodeSet").Index(i).Child("volumeClaimTemplates"), node.VolumeClaimTemplates, pvcImmutableMsg))
		}
	}
	return errs
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

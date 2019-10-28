// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1beta1

import (
	"fmt"
	"net"
	"reflect"

	common "github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	esversion "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/version"
	netutil "github.com/elastic/cloud-on-k8s/pkg/utils/net"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

const (
	cfgInvalidMsg            = "configuration invalid"
	validationFailedMsg      = "Spec validation failed"
	masterRequiredMsg        = "Elasticsearch needs to have at least one master node"
	parseVersionErrMsg       = "Cannot parse Elasticsearch version"
	parseStoredVersionErrMsg = "Cannot parse current Elasticsearch version"
	invalidSanIPErrMsg       = "invalid SAN IP address"
	pvcImmutableMsg          = "Volume claim templates cannot be modified"
	invalidNamesErrMsg       = "Elasticsearch configuration would generate resources with invalid names"
	unsupportedVersionErrMsg = "Unsupported version"
	blacklistedConfigErrMsg  = "Configuration setting is not user-configurable"
	duplicateNodeSets        = "NodeSet names must be unique"
)

type validation func(*Elasticsearch) field.ErrorList

// validations are the validation funcs that apply to creates or updates
var validations = []validation{
	validName,
	hasMaster,
	supportedVersion,
	noBlacklistedSettings,
	validSanIP,
}

type updateValidation func(*Elasticsearch, *Elasticsearch) field.ErrorList

// updateValidations are the validation funcs that only apply to updates
var updateValidations = []updateValidation{
	noDowngrades,
	validUpgradePath,
	pvcModification,
}

// todo sabo convert these to return a field.ErrorList and unroll them all

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
	for _, t := range es.Spec.NodeSets {
		cfg, err := UnpackConfig(t.Config)
		if err != nil {
			// TODO sabo change this to config invalid
			errs = append(errs, field.Invalid(field.NewPath("spec").Child("nodeSets"), es.Name, masterRequiredMsg))
		}
		hasMaster = hasMaster || (cfg.Node.Master && t.Count > 0)
	}
	if !hasMaster {
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("nodeSets"), es.Name, masterRequiredMsg))
	}
	return errs
}

func noBlacklistedSettings(es *Elasticsearch) field.ErrorList {
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
		forbidden := config.HasKeys(SettingsBlacklist)
		for _, setting := range forbidden {
			errs = append(errs, field.Invalid(field.NewPath("spec").Child("nodeSets").Index(i).Child("config"), setting, blacklistedConfigErrMsg))
		}
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
	// todo sabo this seems goofy, do we need another map?
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

func getNode(name string, es *Elasticsearch) *NodeSet {
	for i := range es.Spec.NodeSets {
		if es.Spec.NodeSets[i].Name == name {
			return &es.Spec.NodeSets[i]
		}
	}
	return nil
}

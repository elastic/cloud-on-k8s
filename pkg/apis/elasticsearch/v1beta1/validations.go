// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1beta1

import (
	"errors"
	"fmt"
	"net"
	"reflect"
	"strconv"
	"strings"

	common "github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"

	// "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/name"
	esversion "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/version"
	netutil "github.com/elastic/cloud-on-k8s/pkg/utils/net"
	"github.com/elastic/cloud-on-k8s/pkg/utils/set"
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

// supportedVersion checks if the version is supported.
func supportedVersion(es *Elasticsearch) field.ErrorList {
	var errs field.ErrorList
	ver, err := version.Parse(es.Spec.Version)
	if err != nil {
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("version"), es.Spec.Version, parseVersionErrMsg))
		return errs
	}
	if v := esversion.SupportedVersions(*ver); v != nil {
		if err := v.Supports(*ver); err != nil {
			errs = append(errs, field.Invalid(field.NewPath("spec").Child("version"), es.Spec.Version, unsupportedVersionErrMsg))
		}
	}
	// TODO sabo update tests to look for this message
	return errs
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

// todo sabo add comment and update this to return a list of errors
// func noBlacklistedSettings(es *Elasticsearch) *field.Error {
// 	violations := make(map[int]set.StringSet)
// 	for i, n := range es.Spec.NodeSets {
// 		if n.Config == nil {
// 			continue
// 		}
// 		config, err := common.NewCanonicalConfigFrom(n.Config.Data)
// 		if err != nil {
// 			violations[i] = map[string]struct{}{
// 				cfgInvalidMsg: {},
// 			}
// 			continue
// 		}
// 		forbidden := config.HasKeys(SettingsBlacklist)
// 		// remove duplicates
// 		set := set.Make(forbidden...)
// 		if set.Count() > 0 {
// 			violations[i] = set
// 		}
// 	}
// 	if len(violations) == 0 {
// 		return nil
// 	}
// 	var sb strings.Builder
// 	var sep string
// 	// iterate again to build validation message in node order
// 	for i := range es.Spec.NodeSets {
// 		vs := violations[i]
// 		if vs == nil {
// 			continue
// 		}
// 		sb.WriteString(sep)
// 		sb.WriteString("node[")
// 		sb.WriteString(strconv.FormatInt(int64(i), 10))
// 		sb.WriteString("]: ")
// 		var sep2 string
// 		list := vs.AsSlice()
// 		list.Sort()
// 		for _, msg := range list {
// 			sb.WriteString(sep2)
// 			sb.WriteString(msg)
// 			sep2 = ", "
// 		}
// 		sep = "; "
// 	}
// 	// sb.WriteString(" is not user configurable")
// 	// todo sabo how to make this so we give it the path to the correct config value that is wrong? also update the message
// 	// guessing we need to update the string builder
// 	return field.Invalid(field.NewPath("spec").Child("nodes", "config"), es.Spec.NodeSets[0].Config, blacklistedConfigErrMsg)
// 	// return validation.Result{
// 	// 	Allowed: false,
// 	// 	Reason:  sb.String(),
// 	// }
// }

// TODO sabo how do we unroll this? it has a different signature than the rest. we could update the rest to return a list but that seems like a smell. theres not really a great way to wrap this one either
func betterblacklist(es *Elasticsearch) field.ErrorList {
	var errs []*field.Error
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

func noBlacklistedSettings(es *Elasticsearch) field.ErrorList {
	var errs field.ErrorList
	violations := make(map[int]set.StringSet)
	for i, n := range es.Spec.NodeSets {
		if n.Config == nil {
			continue
		}
		config, err := common.NewCanonicalConfigFrom(n.Config.Data)
		if err != nil {
			violations[i] = map[string]struct{}{
				cfgInvalidMsg: {},
			}
			continue
		}
		forbidden := config.HasKeys(SettingsBlacklist)
		// remove duplicates
		set := set.Make(forbidden...)
		if set.Count() > 0 {
			violations[i] = set
		}
	}
	if len(violations) == 0 {
		return nil
	}
	var sb strings.Builder
	var sep string
	// iterate again to build validation message in node order
	for i := range es.Spec.NodeSets {
		vs := violations[i]
		if vs == nil {
			continue
		}
		sb.WriteString(sep)
		sb.WriteString("node[")
		sb.WriteString(strconv.FormatInt(int64(i), 10))
		sb.WriteString("]: ")
		var sep2 string
		list := vs.AsSlice()
		list.Sort()
		for _, msg := range list {
			sb.WriteString(sep2)
			sb.WriteString(msg)
			sep2 = ", "
		}
		sep = "; "
	}
	sb.WriteString(" is not user configurable")
	return append(errs, field.Invalid(field.NewPath("spec").Child("nodeSets", "config"), es.Spec.NodeSets[0].Config, blacklistedConfigErrMsg))
	// return validation.Result{
	// 	Allowed: false,
	// 	Reason:  sb.String(),
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
func pvcModification(old, current *Elasticsearch) field.ErrorList {
	var errs field.ErrorList
	for i, node := range old.Spec.NodeSets {
		currNode := getNode(node.Name, current)
		if currNode == nil {
			// this is a new sset, so there is nothing to check
			continue
		}

		// ssets do not allow modifications to fields other than 'replicas', 'template', and 'updateStrategy'
		// reflection isn't ideal, but okay here since the ES object does not have the status of the claims
		if !reflect.DeepEqual(node.VolumeClaimTemplates, currNode.VolumeClaimTemplates) {
			// TODO sabo this does not correctly have the right path, we really need the index in _new_ spec
			errs = append(errs, field.Invalid(field.NewPath("spec").Child("nodeSet").Index(i).Child("volumeClaimTemplates"), currNode.VolumeClaimTemplates, pvcImmutableMsg))
		}
	}
	return nil
}

// TODO sabo does this still make sense? im not sure it does
func specUpdatedToBeta() error {
	oldAPIVersion := "elasticsearch.k8s.elastic.co/v1alpha1"

	es := Elasticsearch{}
	if es.APIVersion == oldAPIVersion {
		// return validation.Result{Reason: fmt.Sprintf("%s: outdated APIVersion", validationFailedMsg)}
		return errors.New("")
	}

	if len(es.Spec.NodeSets) == 0 {
		// return validation.Result{Reason: fmt.Sprintf("%s: at least one nodeSet must be defined", validationFailedMsg)}
		return errors.New("")
	}

	for _, set := range es.Spec.NodeSets {
		if set.Count == 0 {
			// msg := fmt.Sprintf("node count of node set '%s' should not be zero", set.Name)
			// return validation.Result{Reason: fmt.Sprintf("%s: %s", validationFailedMsg, msg)}
			return errors.New("")
		}
	}

	return nil
}

func getNode(name string, es *Elasticsearch) *NodeSet {
	for i := range es.Spec.NodeSets {
		if es.Spec.NodeSets[i].Name == name {
			return &es.Spec.NodeSets[i]
		}
	}
	return nil
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1beta1

import (
	"errors"
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
	masterRequiredMsg        = "Elasticsearch needs to have at least one master node"
	parseVersionErrMsg       = "Cannot parse Elasticsearch version"
	parseStoredVersionErrMsg = "Cannot parse current Elasticsearch version"
	invalidSanIPErrMsg       = "invalid SAN IP address"
	pvcImmutableMsg          = "Volume claim templates cannot be modified"
	invalidNamesErrMsg       = "Elasticsearch configuration would generate resources with invalid names"
	unsupportedVersionErrMsg = "Unsupported version"
	blacklistedConfigErrMsg  = "Configuration setting is not user-configurable"
)

type validation func(*Elasticsearch) *field.Error
type updateValidation func(*Elasticsearch, *Elasticsearch) *field.Error

// validations are the validation funcs that apply to creates or updates
var validations = []validation{
	validName,
	hasMaster,
	supportedVersion,
	noBlacklistedSettings,
	validSanIP,
}

// updateValidations are the validation funcs that only apply to updates
var updateValidations = []updateValidation{
	noDowngrades,
	validUpgradePath,
	pvcModification,
}

// validName checks whether the name is valid.
// TODO SABO fix this
func validName(es *Elasticsearch) *field.Error {
	if err := validateNames(&es); err != nil {
		return field.Invalid(field.NewPath("metadata").Child("name"), es.Name, invalidNamesErrMsg)
	}
	return nil
}

// supportedVersion checks if the version is supported.
func supportedVersion(es *Elasticsearch) *field.Error {
	ver, err := version.Parse(es.Spec.Version)
	if err != nil {
		return field.Invalid(field.NewPath("spec").Child("version"), es.Spec.Version, parseVersionErrMsg)
	}
	if v := esversion.SupportedVersions(*ver); v != nil {
		if err := v.Supports(*ver); err == nil {
			return nil
		}
	}
	// TODO sabo update tests to look for this message
	return field.Invalid(field.NewPath("spec").Child("version"), es.Spec.Version, unsupportedVersionErrMsg)
}

// hasMaster checks if the given Elasticsearch cluster has at least one master node.
func hasMaster(es *Elasticsearch) *field.Error {
	var hasMaster bool
	for _, t := range es.Spec.Nodes {
		cfg, err := UnpackConfig(t.Config)
		if err != nil {
			return field.Invalid(field.NewPath("spec").Child("nodes"), es.Name, masterRequiredMsg)
		}
		hasMaster = hasMaster || (cfg.Node.Master && t.NodeCount > 0)
	}
	if hasMaster {
		return nil
	}
	return field.Invalid(field.NewPath("spec").Child("nodes"), es.Name, masterRequiredMsg)
}

// todo sabo add comment and update this to return a list of errors
func noBlacklistedSettings(es *Elasticsearch) *field.Error {
	violations := make(map[int]set.StringSet)
	for i, n := range es.Spec.Nodes {
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
	for i := range es.Spec.Nodes {
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
	// sb.WriteString(" is not user configurable")
	// todo sabo how to make this so we give it the path to the correct config value that is wrong? also update the message
	// guessing we need to update the string builder
	return field.Invalid(field.NewPath("spec").Child("nodes", "config"), es.Spec.Nodes[0].Config, blacklistedConfigErrMsg)
	// return validation.Result{
	// 	Allowed: false,
	// 	Reason:  sb.String(),
	// }
}

func validSanIP(es *Elasticsearch) *field.Error {
	selfSignedCerts := es.Spec.HTTP.TLS.SelfSignedCertificate
	if selfSignedCerts != nil {
		for _, san := range selfSignedCerts.SubjectAlternativeNames {
			if san.IP != "" {
				ip := netutil.MaybeIPTo4(net.ParseIP(san.IP))
				if ip == nil {
					return field.Invalid(field.NewPath("spec").Child("http", "tls", "selfSignedCertificate", "subjectAlternativeNames"), san.IP, invalidSanIPErrMsg)
				}
			}
		}
	}
	return nil
}

// pvcModification ensures no PVCs are changed, as volume claim templates are immutable in stateful sets
func pvcModification(old, current *Elasticsearch) *field.Error {
	if old == nil {
		return nil
	}
	for _, node := range old.Spec.Nodes {
		currNode := getNode(node.Name, current)
		if currNode == nil {
			// this is a new sset, so there is nothing to check
			continue
		}

		// ssets do not allow modifications to fields other than 'replicas', 'template', and 'updateStrategy'
		// reflection isn't ideal, but okay here since the ES object does not have the status of the claims
		if !reflect.DeepEqual(node.VolumeClaimTemplates, currNode.VolumeClaimTemplates) {
			return field.Invalid(field.NewPath("spec").Child("nodes", "volumeClaimTemplates"), currNode.VolumeClaimTemplates, pvcImmutableMsg)
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

func getNode(name string, es *Elasticsearch) *NodeSpec {
	for i := range es.Spec.Nodes {
		if es.Spec.Nodes[i].Name == name {
			return &es.Spec.Nodes[i]
		}
	}
	return nil
}

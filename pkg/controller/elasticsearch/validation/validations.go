// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package validation

import (
	"errors"
	"fmt"
	"net"
	"reflect"
	"strconv"
	"strings"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	common "github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/validation"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
	esversion "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/version"
	netutil "github.com/elastic/cloud-on-k8s/pkg/utils/net"
	"github.com/elastic/cloud-on-k8s/pkg/utils/set"
)

// Validations are all registered Elasticsearch validations.
var Validations = []Validation{
	validName,
	hasMaster,
	supportedVersion,
	noDowngrades,
	validUpgradePath,
	noBlacklistedSettings,
	validSanIP,
	pvcModification,
}

// validName checks whether the name is valid.
func validName(ctx Context) validation.Result {
	if err := name.Validate(ctx.Proposed.Elasticsearch); err != nil {
		return validation.Result{Allowed: false, Reason: invalidName(err)}
	}
	return validation.OK
}

// supportedVersion checks if the version is supported.
func supportedVersion(ctx Context) validation.Result {
	if v := esversion.SupportedVersions(ctx.Proposed.Version); v != nil {
		if err := v.Supports(ctx.Proposed.Version); err == nil {
			return validation.OK
		}
	}
	return validation.Result{Allowed: false, Reason: unsupportedVersion(&ctx.Proposed.Version)}

}

// hasMaster checks if the given Elasticsearch cluster has at least one master node.
func hasMaster(ctx Context) validation.Result {
	var hasMaster bool
	for _, t := range ctx.Proposed.Elasticsearch.Spec.NodeSets {
		cfg, err := v1beta1.UnpackConfig(t.Config)
		if err != nil {
			return validation.Result{Reason: cfgInvalidMsg}
		}
		hasMaster = hasMaster || (cfg.Node.Master && t.Count > 0)
	}
	if hasMaster {
		return validation.OK
	}
	return validation.Result{Reason: masterRequiredMsg}
}

func noBlacklistedSettings(ctx Context) validation.Result {
	violations := make(map[int]set.StringSet)
	for i, n := range ctx.Proposed.Elasticsearch.Spec.NodeSets {
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
		forbidden := config.HasKeys(settings.Blacklist)
		// remove duplicates
		set := set.Make(forbidden...)
		if set.Count() > 0 {
			violations[i] = set
		}
	}
	if len(violations) == 0 {
		return validation.OK
	}
	var sb strings.Builder
	var sep string
	// iterate again to build validation message in node order
	for i := range ctx.Proposed.Elasticsearch.Spec.NodeSets {
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
	return validation.Result{
		Allowed: false,
		Reason:  sb.String(),
	}
}

func validSanIP(ctx Context) validation.Result {
	selfSignedCerts := ctx.Proposed.Elasticsearch.Spec.HTTP.TLS.SelfSignedCertificate
	if selfSignedCerts != nil {
		for _, san := range selfSignedCerts.SubjectAlternativeNames {
			if san.IP != "" {
				ip := netutil.MaybeIPTo4(net.ParseIP(san.IP))
				if ip == nil {
					msg := fmt.Sprintf("%s: %s", invalidSanIPErrMsg, san.IP)
					return validation.Result{
						Error:   errors.New(msg),
						Reason:  msg,
						Allowed: false,
					}
				}
			}
		}
	}
	return validation.OK
}

// pvcModification ensures no PVCs are changed, as volume claim templates are immutable in stateful sets
func pvcModification(ctx Context) validation.Result {
	if ctx.Current == nil {
		return validation.OK
	}
	for _, node := range ctx.Proposed.Elasticsearch.Spec.NodeSets {
		currNodeSet := getNodeSet(node.Name, ctx.Current.Elasticsearch)
		if currNodeSet == nil {
			// this is a new sset, so there is nothing to check
			continue
		}

		// ssets do not allow modifications to fields other than 'replicas', 'template', and 'updateStrategy'
		// reflection isn't ideal, but okay here since the ES object does not have the status of the claims
		if !reflect.DeepEqual(node.VolumeClaimTemplates, currNodeSet.VolumeClaimTemplates) {
			return validation.Result{
				Allowed: false,
				Reason:  pvcImmutableMsg,
			}
		}
	}
	return validation.OK
}

func getNodeSet(name string, es v1beta1.Elasticsearch) *v1beta1.NodeSet {
	for i := range es.Spec.NodeSets {
		if es.Spec.NodeSets[i].Name == name {
			return &es.Spec.NodeSets[i]
		}
	}
	return nil
}

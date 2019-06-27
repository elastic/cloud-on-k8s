// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package validation

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	common "github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/validation"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/driver"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	netutil "github.com/elastic/cloud-on-k8s/operators/pkg/utils/net"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/set"
)

// Validations are all registered Elasticsearch validations.
var Validations = []Validation{
	nameLength,
	hasMaster,
	supportedVersion,
	noDowngrades,
	validUpgradePath,
	noBlacklistedSettings,
	validSanIP,
	tlsCannotBeDisabled,
}

// nameLength checks the length of the Elasticsearch name.
func nameLength(ctx Context) validation.Result {
	if len(ctx.Proposed.Elasticsearch.Name) > name.MaxElasticsearchNameLength {
		return validation.Result{Allowed: false, Reason: fmt.Sprintf(nameTooLongErrMsg, name.MaxElasticsearchNameLength)}
	}
	return validation.OK
}

// supportedVersion checks if the version is supported.
func supportedVersion(ctx Context) validation.Result {
	if v := driver.SupportedVersions(ctx.Proposed.Version); v == nil {
		return validation.Result{Allowed: false, Reason: unsupportedVersion(&ctx.Proposed.Version)}
	}
	return validation.OK
}

// hasMaster checks if the given Elasticsearch cluster has at least one master node.
func hasMaster(ctx Context) validation.Result {
	var hasMaster bool
	for _, t := range ctx.Proposed.Elasticsearch.Spec.Nodes {
		cfg, err := v1alpha1.UnpackConfig(t.Config)
		if err != nil {
			return validation.Result{Reason: cfgInvalidMsg}
		}
		hasMaster = hasMaster || (cfg.Node.Master && t.NodeCount > 0)
	}
	if hasMaster {
		return validation.OK
	}
	return validation.Result{Reason: masterRequiredMsg}
}

func noBlacklistedSettings(ctx Context) validation.Result {
	violations := make(map[int]set.StringSet)
	for i, n := range ctx.Proposed.Elasticsearch.Spec.Nodes {
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
	for i := range ctx.Proposed.Elasticsearch.Spec.Nodes {
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

func tlsCannotBeDisabled(ctx Context) validation.Result {
	if !ctx.Proposed.Elasticsearch.Spec.HTTP.TLS.Enabled() {
		return validation.Result{
			Allowed: false,
			Reason:  "TLS cannot be disabled for Elasticsearch currently",
		}
	}
	return validation.OK

}

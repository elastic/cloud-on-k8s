// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"k8s.io/apimachinery/pkg/util/validation/field"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	common "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/settings"
)

var warnings = []validation{
	noUnsupportedSettings,
}

func noUnsupportedSettings(es esv1.Elasticsearch) field.ErrorList {
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
		errs = append(errs, validateSettings(config, i)...)
		errs = append(errs, validateClientAuthentication(config, i)...)
	}
	return errs
}

func validateSettings(config *common.CanonicalConfig, index int) field.ErrorList {
	var errs field.ErrorList
	unsupported := config.HasKeys(esv1.UnsupportedSettings)
	for _, setting := range unsupported {
		errs = append(errs, field.Forbidden(field.NewPath("spec").Child("nodeSets").Index(index).Child("config").Child(setting), unsupportedConfigErrMsg))
	}
	return errs
}

func validateClientAuthentication(config *common.CanonicalConfig, index int) field.ErrorList {
	forbiddenValue := "required" // we allow 'none' and 'optional' but 'required' is not supported

	var errs field.ErrorList
	value, err := config.String(esv1.XPackSecurityHttpSslClientAuthentication)
	if err != nil {
		return errs
	}
	if value == forbiddenValue {
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("nodeSets").Index(index).Child("config").Child(esv1.XPackSecurityHttpSslClientAuthentication),
			value, unsupportedClientAuthenticationMsg))
	}
	return errs
}

func CheckForWarnings(es esv1.Elasticsearch) error {
	warnings := check(es, warnings)
	if len(warnings) > 0 {
		return warnings.ToAggregate()
	}
	return nil
}

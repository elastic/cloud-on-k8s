// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1

import (
	common "github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

var warnings = []validation{
	noUnsupportedSettings,
}

func noUnsupportedSettings(es *Elasticsearch) field.ErrorList {
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
		unsupported := config.HasKeys(UnsupportedSettings)
		for _, setting := range unsupported {
			errs = append(errs, field.Forbidden(field.NewPath("spec").Child("nodeSets").Index(i).Child("config").Child(setting), unsupportedConfigErrMsg))
		}
	}
	return errs
}

func (r *Elasticsearch) CheckForWarnings() error {
	warnings := r.check(warnings)
	if len(warnings) > 0 {
		return warnings.ToAggregate()
	}
	return nil
}

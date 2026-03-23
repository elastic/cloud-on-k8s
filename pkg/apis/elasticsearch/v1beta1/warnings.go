// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1beta1

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	common "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
)

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

// SettingsWarnings converts noUnsupportedSettings errors into admission warnings
// so they can be surfaced at apply time without rejecting the request.
func SettingsWarnings(es *Elasticsearch) admission.Warnings {
	errs := noUnsupportedSettings(es)
	if len(errs) == 0 {
		return nil
	}
	w := make(admission.Warnings, len(errs))
	for i, e := range errs {
		w[i] = fmt.Sprintf("%s: %s", e.Field, e.Detail)
	}
	return w
}

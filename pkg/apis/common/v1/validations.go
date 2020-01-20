// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1

import (
	"encoding/json"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// NoUnknownFields checks whether the last applied config annotation contains json with unknown fields.
func NoUnknownFields(dest interface{}, meta metav1.ObjectMeta) field.ErrorList {
	var errs field.ErrorList
	if cfg, ok := meta.Annotations[v1.LastAppliedConfigAnnotation]; ok {
		d := json.NewDecoder(strings.NewReader(cfg))
		d.DisallowUnknownFields()
		if err := d.Decode(&dest); err != nil {
			errString := err.Error()
			unknownPrefix := "json: unknown field "
			if strings.HasPrefix(errString, unknownPrefix) {
				fld := strings.TrimPrefix(errString, unknownPrefix)
				errs = append(errs, field.Invalid(field.NewPath(fld), meta.Name, errString))
			}
		}
	}

	return errs
}

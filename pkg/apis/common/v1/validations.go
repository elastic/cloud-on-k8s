// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1

import (
	"encoding/json"
	"fmt"
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
				if len(fld) >= 2 {
					fld = fld[1 : len(fld)-1] // removes quotes from fld
				}
				err := field.Invalid(
					field.NewPath(fld),
					fld,
					fmt.Sprintf("%s field found in %s annotation is unknown", fld, v1.LastAppliedConfigAnnotation))
				errs = append(errs, err)
			}
		}
	}
	return errs
}

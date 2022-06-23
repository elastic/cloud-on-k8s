// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	"encoding/json"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"

	common_name "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
)

// NoUnknownFields checks whether the last applied config annotation contains json with unknown fields.
func NoUnknownFields(dest runtime.Object, meta metav1.ObjectMeta) field.ErrorList {
	var errs field.ErrorList
	//nolint:nestif
	if cfg, ok := meta.Annotations[v1.LastAppliedConfigAnnotation]; ok {
		d := json.NewDecoder(strings.NewReader(cfg))
		d.DisallowUnknownFields()
		// decode in a copy of the resource to be validated to avoid mutation if the object in the annotation is different
		if err := d.Decode(dest.DeepCopyObject()); err != nil {
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
					fmt.Sprintf("%s field found in the %s annotation is unknown. This is often due to incorrect indentation in the manifest.", fld, v1.LastAppliedConfigAnnotation))
				errs = append(errs, err)
			}
		}
	}
	return errs
}

// CheckNameLength checks that the object name does not exceed the maximum length.
func CheckNameLength(obj runtime.Object) field.ErrorList {
	path := field.NewPath("metadata").Child("name")
	accessor := meta.NewAccessor()
	name, err := accessor.Name(obj)
	if err != nil {
		return field.ErrorList{field.InternalError(path, err)}
	}

	if len(name) > common_name.MaxResourceNameLength {
		return field.ErrorList{field.TooLong(path, name, common_name.MaxResourceNameLength)}
	}

	return nil
}

// CheckSupportedStackVersion checks that the given version is a valid Stack version supported by ECK.
func CheckSupportedStackVersion(ver string, supported version.MinMaxVersion) field.ErrorList {
	v, err := ParseVersion(ver)
	if err != nil {
		return err
	}

	if err := supported.WithMin(version.GlobalMinStackVersion).WithinRange(*v); err != nil {
		return field.ErrorList{field.Invalid(field.NewPath("spec").Child("version"), ver, fmt.Sprintf("Unsupported version: %v", err))}
	}

	return nil
}

// CheckNoDowngrade checks current and previous versions to ensure no downgrades are happening.
func CheckNoDowngrade(prev, curr string) field.ErrorList {
	prevVer, err := ParseVersion(prev)
	if err != nil {
		return err
	}

	currVer, err := ParseVersion(curr)
	if err != nil {
		return err
	}

	if !currVer.GTE(*prevVer) {
		return field.ErrorList{field.Forbidden(field.NewPath("spec").Child("version"), "Version downgrades are not supported")}
	}

	return nil
}

// CheckAssociationRefs checks that the given association references are valid.
func CheckAssociationRefs(path *field.Path, refs ...ObjectSelector) field.ErrorList {
	for _, ref := range refs {
		if err := ref.IsValid(); err != nil {
			return field.ErrorList{field.Forbidden(path, fmt.Sprintf("Invalid association reference: %s", err))}
		}
	}
	return nil
}

func ParseVersion(ver string) (*version.Version, field.ErrorList) {
	v, err := version.Parse(ver)
	if err != nil {
		return nil, field.ErrorList{field.Invalid(field.NewPath("spec").Child("version"), ver, fmt.Sprintf("Invalid version: %v", err))}
	}

	return &v, nil
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1beta1

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	esversion "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/version"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

const (
	noDowngradesMsg = "Downgrades are not supported"
)

func unsupportedVersion(v *version.Version) string {
	return fmt.Sprintf("unsupported version: %v", v)
}

func unsupportedUpgradePath(v1, v2 version.Version) string {
	return fmt.Sprintf("unsupported version upgrade from %v to %v", v1, v2)
}

func noDowngrades(old, current *Elasticsearch) field.ErrorList {
	var errs field.ErrorList
	oldVer, err := version.Parse(old.Spec.Version)
	if err != nil {
		// this should not happen, since this is the already persisted version
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("version"), old.Spec.Version, parseStoredVersionErrMsg))
	}
	currVer, err := version.Parse(current.Spec.Version)
	if err != nil {
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("version"), current.Spec.Version, parseVersionErrMsg))
	}
	if len(errs) != 0 {
		return errs
	}
	if !currVer.IsSameOrAfter(*oldVer) {
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("version"), current.Spec.Version, noDowngradesMsg))
	}
	return errs
}

func validUpgradePath(old, current *Elasticsearch) field.ErrorList {
	var errs field.ErrorList
	oldVer, err := version.Parse(old.Spec.Version)
	if err != nil {
		// this should not happen, since this is the already persisted version
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("version"), old.Spec.Version, parseStoredVersionErrMsg))
	}
	currVer, err := version.Parse(current.Spec.Version)
	if err != nil {
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("version"), current.Spec.Version, parseVersionErrMsg))
	}
	if len(errs) != 0 {
		return errs
	}
	// TODO sabo i think we can remove this since we are already checking if it is a supported version?
	v := esversion.SupportedVersions(*currVer)
	if v == nil {
		// TODO sabo make this a constant
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("version"), current.Spec.Version, "Unsupported version"))
		return errs
	}

	err = v.Supports(*oldVer)
	if err != nil {
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("version"), current.Spec.Version, "Unsupported upgrade path"))
	}
	return errs
}

func invalidName(err error) string {
	return fmt.Sprintf("%s: %v", invalidNamesErrMsg, err)
}

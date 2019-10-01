// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

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

// TODO sabo make a new type for transient checks
// TODO sabo do we need to check if old is nil? i dont think so
func noDowngrades(old, current *Elasticsearch) *field.Error {
	if old == nil {
		return nil
	}
	oldVer, err := version.Parse(old.Spec.Version)
	if err != nil {
		return field.Invalid(field.NewPath("spec").Child("version"), old.Spec.Version, parseStoredVersionErrMsg)
	}
	currVer, err := version.Parse(current.Spec.Version)
	if err != nil {
		return field.Invalid(field.NewPath("spec").Child("version"), current.Spec.Version, parseVersionErrMsg)
	}
	if !currVer.IsSameOrAfter(*oldVer) {
		return field.Invalid(field.NewPath("spec").Child("version"), current.Spec.Version, noDowngradesMsg)
	}
	return nil
}

func validUpgradePath(old, current *Elasticsearch) *field.Error {
	if old == nil {
		return nil
	}
	oldVer, err := version.Parse(old.Spec.Version)
	if err != nil {
		return field.Invalid(field.NewPath("spec").Child("version"), old.Spec.Version, parseStoredVersionErrMsg)
	}
	currVer, err := version.Parse(current.Spec.Version)
	if err != nil {
		return field.Invalid(field.NewPath("spec").Child("version"), current.Spec.Version, parseVersionErrMsg)
	}
	// TODO sabo i think we can remove this since we are already checking if it is a supported version?
	v := esversion.SupportedVersions(*currVer)
	if v == nil {
		// TODO sabo make this a constant
		return field.Invalid(field.NewPath("spec").Child("version"), current.Spec.Version, "Unsupported version")
	}

	err = v.Supports(*oldVer)
	if err != nil {
		return field.Invalid(field.NewPath("spec").Child("version"), current.Spec.Version, "Unsupported upgrade path")
	}
	return nil
}

func invalidName(err error) string {
	return fmt.Sprintf("%s: %v", invalidNamesErrMsg, err)
}

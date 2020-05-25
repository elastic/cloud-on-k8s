// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	pkgerrors "github.com/pkg/errors"
)

// OperatorLicenseType is the type of operator level license a resource is describing.
type OperatorLicenseType string

const (
	LicenseTypeEnterprise      OperatorLicenseType = "enterprise"
	LicenseTypeEnterpriseTrial OperatorLicenseType = "enterprise_trial"
	// LicenseTypeLegacyTrial earlier versions of ECK used this as the trial identifier
	LicenseTypeLegacyTrial OperatorLicenseType = "enterprise-trial"
)

type ElasticsearchLicense struct {
	License client.License `json:"license"`
}

type EnterpriseLicense struct {
	License LicenseSpec `json:"license"`
}

type LicenseSpec struct {
	Status             string                 `json:"status,omitempty"`
	UID                string                 `json:"uid"`
	Type               OperatorLicenseType    `json:"type"`
	IssueDate          *time.Time             `json:"issue_date,omitempty"`
	IssueDateInMillis  int64                  `json:"issue_date_in_millis"`
	ExpiryDate         *time.Time             `json:"expiry_date,omitempty"`
	ExpiryDateInMillis int64                  `json:"expiry_date_in_millis"`
	MaxInstances       int                    `json:"max_instances,omitempty"`
	MaxResourceUnits   int                    `json:"max_resource_units,omitempty"`
	IssuedTo           string                 `json:"issued_to"`
	Issuer             string                 `json:"issuer"`
	StartDateInMillis  int64                  `json:"start_date_in_millis"`
	Signature          string                 `json:"signature,omitempty"`
	ClusterLicenses    []ElasticsearchLicense `json:"cluster_licenses"`
	Version            int                    // not marshalled but part of the signature
}

// EnterpriseLicenseTypeOrder license types mapped to ints in increasing order of feature sets for sorting purposes.
var EnterpriseLicenseTypeOrder = map[OperatorLicenseType]int{
	LicenseTypeLegacyTrial:     0,
	LicenseTypeEnterpriseTrial: 1,
	LicenseTypeEnterprise:      2,
}

// StartTime is the date as of which this license is valid.
func (l EnterpriseLicense) StartTime() time.Time {
	return time.Unix(0, l.License.StartDateInMillis*int64(time.Millisecond))
}

// ExpiryTime is the date as of which the license is no longer valid.
func (l EnterpriseLicense) ExpiryTime() time.Time {
	return time.Unix(0, l.License.ExpiryDateInMillis*int64(time.Millisecond))
}

// IsValid returns true if the license is still valid at the given point in time.
func (l EnterpriseLicense) IsValid(instant time.Time) bool {
	return (l.StartTime().Equal(instant) || l.StartTime().Before(instant)) &&
		l.ExpiryTime().After(instant)
}

// IsTrial returns true if this is a self-generated trial license.
func (l EnterpriseLicense) IsTrial() bool {
	return l.License.Type == LicenseTypeEnterpriseTrial || l.License.Type == LicenseTypeLegacyTrial
}

// IsECKManagedTrial returns true if this license has been issued by ECK or if this is an empty license that ECK can fill in.
func (l EnterpriseLicense) IsECKManagedTrial() bool {
	return l.IsTrial() && (l.License.Issuer == ECKLicenseIssuer || l.License.Issuer == "")
}

// IsMissingFields returns an error if any of the required fields are missing. Expected state on trial licenses.
func (l EnterpriseLicense) IsMissingFields() error {
	var missing []string
	if l.License.Issuer == "" {
		missing = append(missing, "spec.issuer")
	}
	if l.License.IssuedTo == "" {
		missing = append(missing, "spec.issued_to")
	}
	if l.License.ExpiryDateInMillis == 0 {
		missing = append(missing, "spec.expiry_date_in_millis")
	}
	if l.License.StartDateInMillis == 0 {
		missing = append(missing, "spec.start_date_in_millis")
	}
	if l.License.IssueDateInMillis == 0 {
		missing = append(missing, "spec.issue_date_in_millis")
	}
	if l.License.UID == "" {
		missing = append(missing, "spec.uid")
	}
	if len(missing) > 0 {
		return pkgerrors.Errorf("required fields are missing: %v", missing)
	}
	return nil
}

// LicenseStatus expresses the validity status of a license.
type LicenseStatus string

// Supported LicenseStatus values.
const (
	LicenseStatusValid   LicenseStatus = "Valid"
	LicenseStatusExpired LicenseStatus = "Expired"
	LicenseStatusInvalid LicenseStatus = "Invalid"
)

var _ Signable = &EnterpriseLicense{}

/*
 * Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
 * or more contributor license agreements. Licensed under the Elastic License;
 * you may not use this file except in compliance with the Elastic License.
 */

package license

import (
	"fmt"
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
)

type SourceClusterLicense struct {
	License client.License `json:"license"`
}

type SourceEnterpriseLicense struct {
	Data SourceLicenseData `json:"license"`
}

type SourceLicenseData struct {
	Status             string                 `json:"status,omitempty"`
	UID                string                 `json:"uid"`
	Type               string                 `json:"type"`
	IssueDate          *time.Time             `json:"issue_date,omitempty"`
	IssueDateInMillis  int64                  `json:"issue_date_in_millis"`
	ExpiryDate         *time.Time             `json:"expiry_date,omitempty"`
	ExpiryDateInMillis int64                  `json:"expiry_date_in_millis"`
	MaxInstances       int                    `json:"max_instances"`
	IssuedTo           string                 `json:"issued_to"`
	Issuer             string                 `json:"issuer"`
	StartDateInMillis  int64                  `json:"start_date_in_millis"`
	Signature          string                 `json:"signature,omitempty"`
	ClusterLicenses    []SourceClusterLicense `json:"cluster_licenses"`
}

// StartTime is the date as of which this license is valid.
func (l SourceEnterpriseLicense) StartTime() time.Time {
	return time.Unix(0, l.Data.StartDateInMillis*int64(time.Millisecond))
}

// ExpiryTime is the date as of which the license is no longer valid.
func (l SourceEnterpriseLicense) ExpiryTime() time.Time {
	return time.Unix(0, l.Data.ExpiryDateInMillis*int64(time.Millisecond))
}

// IsValid returns true if the license is still valid at the given point in time.
func (l SourceEnterpriseLicense) IsValid(instant time.Time) bool {
	return (l.StartTime().Equal(instant) || l.StartTime().Before(instant)) &&
		l.ExpiryTime().After(instant)
}

// IsTrial returns true if this is a self-generated trial license.
func (l SourceEnterpriseLicense) IsTrial() bool {
	return l.Data.Type == string(v1alpha1.LicenseTypeEnterpriseTrial)
}

// IsMissingFields returns an error if any of the required fields are missing. Expected state on trial licenses.
func (l SourceEnterpriseLicense) IsMissingFields() error {
	var missing []string
	if l.Data.Issuer == "" {
		missing = append(missing, "spec.issuer")
	}
	if l.Data.IssuedTo == "" {
		missing = append(missing, "spec.issued_to")
	}
	if l.Data.ExpiryDateInMillis == 0 {
		missing = append(missing, "spec.expiry_date_in_millis")
	}
	if l.Data.StartDateInMillis == 0 {
		missing = append(missing, "spec.start_date_in_millis")
	}
	if l.Data.IssueDateInMillis == 0 {
		missing = append(missing, "spec.issue_date_in_millis")
	}
	if l.Data.UID == "" {
		missing = append(missing, "spec.uid")
	}
	if len(missing) > 0 {
		return fmt.Errorf("required fields are missing: %v", missing)
	}
	return nil
}

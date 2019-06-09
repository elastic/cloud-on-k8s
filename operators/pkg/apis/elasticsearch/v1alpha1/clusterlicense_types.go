// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// License common interface for licenses.
type License interface {
	StartTime() time.Time
	ExpiryDate() time.Time
}

// LicenseType the type of a license.
type LicenseType string

// Supported LicenseTypes
const (
	LicenseTypeBasic    LicenseType = "basic"
	LicenseTypeTrial    LicenseType = "trial"
	LicenseTypeGold     LicenseType = "gold"
	LicenseTypePlatinum LicenseType = "platinum"
)

// LicenseTypeOrder license types mapped to ints in increasing order of feature sets for sorting purposes.
var LicenseTypeOrder = map[LicenseType]int{
	LicenseTypeBasic:    1,
	LicenseTypeTrial:    2,
	LicenseTypeGold:     3,
	LicenseTypePlatinum: 4,
}

// LicenseTypeFromString converts a given string to a license type if possible.
// If the string is empty, default to a basic license.
func LicenseTypeFromString(s string) (LicenseType, error) {
	if s == "" {
		return LicenseTypeBasic, nil
	}
	licenseType := LicenseType(s)
	_, exists := LicenseTypeOrder[licenseType]
	if !exists {
		return "", fmt.Errorf("invalid license type: %s", s)
	}
	return licenseType, nil
}

// IsGoldOrPlatinum returns true if the license is gold or platinum,
// hence probably requires some special treatment.
func (l LicenseType) IsGoldOrPlatinum() bool {
	switch l {
	case LicenseTypeGold, LicenseTypePlatinum:
		return true
	default:
		return false
	}
}

// String returns the string representation of the license type
func (l LicenseType) String() string {
	return string(l)
}

// LicenseMeta contains license (meta) information shared between enterprise and cluster licenses.
type LicenseMeta struct {
	// UID is the license UID not the k8s API UID (!)
	UID                string `json:"uid,omitempty"`
	IssueDateInMillis  int64  `json:"issueDateInMillis,omitempty"`
	ExpiryDateInMillis int64  `json:"expiryDateInMillis,omitempty"`
	IssuedTo           string `json:"issuedTo,omitempty"`
	Issuer             string `json:"issuer,omitempty"`
	StartDateInMillis  int64  `json:"startDateInMillis,omitempty"`
}

// StartTime is the date as of which this license is valid.
func (l LicenseMeta) StartTime() time.Time {
	return time.Unix(0, l.StartDateInMillis*int64(time.Millisecond))
}

// ExpiryTime is the date as of which the license is no longer valid.
func (l LicenseMeta) ExpiryDate() time.Time {
	return time.Unix(0, l.ExpiryDateInMillis*int64(time.Millisecond))
}

// IsValid returns true if the license is still valid at the given point in time.
func (l LicenseMeta) IsValid(instant time.Time) bool {
	return (l.StartTime().Equal(instant) || l.StartTime().Before(instant)) &&
		l.ExpiryDate().After(instant)
}

type LicenseStatus string

const (
	LicenseStatusValid   LicenseStatus = "Valid"
	LicenseStatusExpired LicenseStatus = "Expired"
	LicenseStatusInvalid LicenseStatus = "Invalid"
)

// ClusterLicenseSpec defines the desired state of ClusterLicense
type ClusterLicenseSpec struct {
	LicenseMeta  `json:",inline"`
	MaxNodes     int                      `json:"maxNodes"`
	Type         LicenseType              `json:"type"`
	SignatureRef corev1.SecretKeySelector `json:"signatureRef"`
}

// IsEmpty returns true if this spec is empty.
func (cls ClusterLicenseSpec) IsEmpty() bool {
	return cls == ClusterLicenseSpec{}
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// License common interface for licenses.
type License interface {
	StartDate() time.Time
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
	UID                string `json:"uid"`
	IssueDateInMillis  int64  `json:"issueDateInMillis"`
	ExpiryDateInMillis int64  `json:"expiryDateInMillis"`
	IssuedTo           string `json:"issuedTo"`
	Issuer             string `json:"issuer"`
	StartDateInMillis  int64  `json:"startDateInMillis"`
}

// StartDate is the date as of which this license is valid.
func (l LicenseMeta) StartDate() time.Time {
	return time.Unix(0, l.StartDateInMillis*int64(time.Millisecond))
}

// ExpiryDate is the date as of which the license is no longer valid.
func (l LicenseMeta) ExpiryDate() time.Time {
	return time.Unix(0, l.ExpiryDateInMillis*int64(time.Millisecond))
}

// IsValid returns true if the license is still valid at the given point in time.
func (l LicenseMeta) IsValid(instant time.Time) bool {
	return (l.StartDate().Equal(instant) || l.StartDate().Before(instant)) &&
		l.ExpiryDate().After(instant)
}

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

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterLicense is the Schema for the clusterlicenses API
// +kubebuilder:resource:shortName=cl
// +k8s:openapi-gen=true
type ClusterLicense struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ClusterLicenseSpec `json:"spec,omitempty"`
}

// StartDate is the date as of which this license is valid.
func (l *ClusterLicense) StartDate() time.Time {
	return l.Spec.StartDate()
}

// ExpiryDate is the date as of which the license is no longer valid.
func (l *ClusterLicense) ExpiryDate() time.Time {
	return l.Spec.ExpiryDate()
}

// IsEmpty returns true if this license has an empty spec.
func (l ClusterLicense) IsEmpty() bool {
	return l.Spec.IsEmpty()
}

// IsValid returns true if the license is still valid at the given point in time.
func (l ClusterLicense) IsValid(instant time.Time) bool {
	return l.Spec.IsValid(instant)
}

var _ License = &ClusterLicense{}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterLicenseList contains a list of ClusterLicense
type ClusterLicenseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterLicense `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterLicense{}, &ClusterLicenseList{})
}

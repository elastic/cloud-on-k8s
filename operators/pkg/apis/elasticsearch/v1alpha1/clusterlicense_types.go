// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import (
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

const (
	LicenseTypeStandard LicenseType = "standard"
	LicenseTypeGold     LicenseType = "gold"
	LicenseTypePlatinum LicenseType = "platinum"
)

// LicenseTypeOrder license types mapped to ints in increasing order of feature sets for sorting purposes.
var LicenseTypeOrder = map[LicenseType]int{
	// default value 0 for invalid types
	LicenseTypeStandard: 1,
	LicenseTypeGold:     2,
	LicenseTypePlatinum: 3,
}

// LicenseTypeFromString converts a given string to a license type if possible.
func LicenseTypeFromString(s string) LicenseType {
	var res LicenseType
	if LicenseTypeOrder[LicenseType(s)] > 0 {
		res = LicenseType(s)
	}
	return res
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

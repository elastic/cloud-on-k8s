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

// EnterpriseLicenseType is the type of enterprise license a resource is describing.
type EnterpriseLicenseType string

const (
	LicenseTypeEnterprise      EnterpriseLicenseType = "enterprise"
	LicenseTypeEnterpriseTrial EnterpriseLicenseType = "enterprise-trial"
)

// EulaState defines whether or not a user has accepted the end user license agreement.
type EulaState struct {
	Accepted bool `json:"accepted"`
}

// EnterpriseLicenseSpec defines the desired state of EnterpriseLicense
type EnterpriseLicenseSpec struct {
	LicenseMeta  `json:",inline"`
	Type         EnterpriseLicenseType    `json:"type"`
	MaxInstances int                      `json:"maxInstances,omitempty"`
	SignatureRef corev1.SecretKeySelector `json:"signatureRef,omitempty"`
	// +optional
	ClusterLicenseSpecs []ClusterLicenseSpec `json:"clusterLicenses,omitempty"`
	Eula                EulaState            `json:"eula"`
}

// EnterpriseLicenseStatus defines the current status of the license. Informational only, maybe empty.
type EnterpriseLicenseStatus struct {
	LicenseStatus LicenseStatus `json:"status,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// EnterpriseLicense is the Schema for the enterpriselicenses API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="status",type="string",JSONPath=".status.status"
// +kubebuilder:resource:shortName=el
type EnterpriseLicense struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EnterpriseLicenseSpec   `json:"spec,omitempty"`
	Status EnterpriseLicenseStatus `json:"status,omitempty"`
}

// StartDate is the date as of which this license is valid.
func (l *EnterpriseLicense) StartDate() time.Time {
	return l.Spec.StartDate()
}

// ExpiryDate is the date as of which the license is no longer valid.
func (l *EnterpriseLicense) ExpiryDate() time.Time {
	return l.Spec.ExpiryDate()
}

// IsMissingFields returns an error if any of the required fields are missing. Expected state on trial licenses.
func (l EnterpriseLicense) IsMissingFields() error {
	var missing []string
	if l.Spec.Issuer == "" {
		missing = append(missing, "spec.issuer")
	}
	if l.Spec.IssuedTo == "" {
		missing = append(missing, "spec.issued_to")
	}
	if l.Spec.ExpiryDateInMillis == 0 {
		missing = append(missing, "spec.expiry_date_in_millis")
	}
	if l.Spec.StartDateInMillis == 0 {
		missing = append(missing, "spec.start_date_in_millis")
	}
	if l.Spec.IssueDateInMillis == 0 {
		missing = append(missing, "spec.issue_date_in_millis")
	}
	if l.Spec.UID == "" {
		missing = append(missing, "spec.uid")
	}
	if len(missing) > 0 {
		return fmt.Errorf("required fields are missing: %v", missing)
	}
	return nil
}

// IsValid returns true if the license is still valid at the given point in time.
func (l EnterpriseLicense) IsValid(instant time.Time) bool {
	return l.Spec.IsValid(instant)
}

func (l EnterpriseLicense) IsTrial() bool {
	return l.Spec.Type == LicenseTypeEnterpriseTrial
}

var _ License = &EnterpriseLicense{}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// EnterpriseLicenseList contains a list of EnterpriseLicense
type EnterpriseLicenseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EnterpriseLicense `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EnterpriseLicense{}, &EnterpriseLicenseList{})
}

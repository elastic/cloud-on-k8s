// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EnterpriseLicenseSpec defines the desired state of EnterpriseLicense
type EnterpriseLicenseSpec struct {
	LicenseMeta  `json:",inline"`
	Type         string                   `json:"type"`
	MaxInstances int                      `json:"maxInstances"`
	SignatureRef corev1.SecretKeySelector `json:"signatureRef"`
	// +optional
	ClusterLicenseSpecs []ClusterLicenseSpec `json:"clusterLicenses,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// EnterpriseLicense is the Schema for the enterpriselicenses API
// +k8s:openapi-gen=true
// +kubebuilder:resource:shortName=el
type EnterpriseLicense struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec EnterpriseLicenseSpec `json:"spec,omitempty"`
}

// StartDate is the date as of which this license is valid.
func (l *EnterpriseLicense) StartDate() time.Time {
	return l.Spec.StartDate()
}

// ExpiryDate is the date as of which the license is no longer valid.
func (l *EnterpriseLicense) ExpiryDate() time.Time {
	return l.Spec.ExpiryDate()
}

// IsValid returns true if the license is still valid at the given point in time.
func (l EnterpriseLicense) IsValid(instant time.Time) bool {
	return l.Spec.IsValid(instant)
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

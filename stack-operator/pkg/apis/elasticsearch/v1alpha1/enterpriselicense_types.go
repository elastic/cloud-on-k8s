package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EnterpriseLicenseSpec defines the desired state of EnterpriseLicense
type EnterpriseLicenseSpec struct {
	// TODO unify (embed?) with cluster license
	UID                string                 `json:"uid"`
	Type               string                 `json:"type"`
	IssueDateInMillis  int64                  `json:"issueDateInMillis"`
	ExpiryDateInMillis int64                  `json:"expiryDateInMillis"`
	MaxNodes           int                    `json:"maxNodes"`
	IssuedTo           string                 `json:"issuedTo"`
	Issuer             string                 `json:"issuer"`
	StartDateInMillis  int64                  `json:"startDateInMillis"`
	SignatureRef       corev1.SecretReference `json:"signatureRef"`
	// +optional
	ClusterLicenses    []ClusterLicense       `json:"clusterLicenses,omitempty"`
}

// EnterpriseLicenseStatus defines the observed state of EnterpriseLicense
type EnterpriseLicenseStatus struct {
	LicenseStatus string `json:"licenseStatus"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// EnterpriseLicense is the Schema for the enterpriselicenses API
// +k8s:openapi-gen=true
type EnterpriseLicense struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EnterpriseLicenseSpec   `json:"spec,omitempty"`
	Status EnterpriseLicenseStatus `json:"status,omitempty"`
}

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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterLicenseSpec defines the desired state of ClusterLicense
type ClusterLicenseSpec struct {
	// UID is the license UID not the k8s API UID (!)
	UID                string                 `json:"uid"`
	Type               string                 `json:"type"`
	IssueDateInMillis  int64                  `json:"issueDateInMillis"`
	ExpiryDateInMillis int64                  `json:"expiryDateInMillis"`
	MaxNodes           int                    `json:"maxNodes"`
	IssuedTo           string                 `json:"issuedTo"`
	Issuer             string                 `json:"issuer"`
	StartDateInMillis  int64                  `json:"startDateInMillis"`
	SignatureRef       corev1.SecretReference `json:"signatureRef"`
}

// IsEmpty returns true if this spec is empty.
func (cls ClusterLicenseSpec) IsEmpty() bool {
	return cls == ClusterLicenseSpec{}
}

// ClusterLicenseStatus defines the observed state of ClusterLicense
type ClusterLicenseStatus struct {
	LicenseStatus string `json:"licenseStatus"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterLicense is the Schema for the clusterlicenses API
// +k8s:openapi-gen=true
type ClusterLicense struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterLicenseSpec   `json:"spec,omitempty"`
	Status ClusterLicenseStatus `json:"status,omitempty"`
}

// IsEmpty returns true if this license has an empty spec.
func (cl ClusterLicense) IsEmpty() bool {
	return cl.Spec.IsEmpty()
}

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

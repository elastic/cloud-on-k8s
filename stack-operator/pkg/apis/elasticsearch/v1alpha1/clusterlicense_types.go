package v1alpha1

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type License interface {
	StartDate() time.Time
	ExpiryDate() time.Time
}

type LicenseType string

const (
	LicenseTypeStandard LicenseType = "standard"
	LicenseTypeGold     LicenseType = "gold"
	LicenseTypePlatinum LicenseType = "platinum"
)

var LicenseTypeOrder = map[LicenseType]int{
	LicenseTypeStandard: 1, // default value 0 for invalid types
	LicenseTypeGold:     2,
	LicenseTypePlatinum: 3,
}

func LicenseTypeFromString(s string) *LicenseType {
	var res LicenseType
	if LicenseTypeOrder[LicenseType(s)] > 0 {
		res = LicenseType(s)
	}
	return &res
}

// ClusterLicenseSpec defines the desired state of ClusterLicense
type ClusterLicenseSpec struct {
	// UID is the license UID not the k8s API UID (!)
	UID                string                 `json:"uid"`
	Type               LicenseType            `json:"type"`
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

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterLicense is the Schema for the clusterlicenses API
// +k8s:openapi-gen=true
type ClusterLicense struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ClusterLicenseSpec `json:"spec,omitempty"`
}

func (l *ClusterLicense) StartDate() time.Time {
	return time.Unix(0, l.Spec.StartDateInMillis*int64(time.Millisecond))
}

func (l *ClusterLicense) ExpiryDate() time.Time {
	return time.Unix(0, l.Spec.ExpiryDateInMillis*int64(time.Millisecond))
}

// IsEmpty returns true if this license has an empty spec.
func (l ClusterLicense) IsEmpty() bool {
	return l.Spec.IsEmpty()
}

type SafetyMargin struct {
	ValidSince time.Duration
	ValidFor   time.Duration
}

func NewSafetyMargin() SafetyMargin {
	return SafetyMargin{}
}

func (l ClusterLicense) IsValidAt(instant time.Time, margin SafetyMargin) bool {
	return l.StartDate().Add(margin.ValidSince).Before(instant) &&
		l.ExpiryDate().Add(-1*margin.ValidFor).After(instant)
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

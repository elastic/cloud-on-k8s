package v1alpha1

import (
	"bytes"
	"encoding/json"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type License interface {
	StartDate() time.Time
	ExpiryDate() time.Time
}

// TODO not sure being able to do numeric comparisons is worth  all the grovelling that follows
type LicenseType int

const (
	LicenseTypeStandard LicenseType = iota + 1
	LicenseTypeGold
	LicenseTypePlatinum
)

var licenseTypeToString = map[LicenseType]string{
	LicenseTypeStandard: "standard",
	LicenseTypeGold:     "gold",
	LicenseTypePlatinum: "platinum",
}

var LicenseTypeFromString = map[string]LicenseType{
	"standard": LicenseTypeStandard,
	"gold":     LicenseTypeGold,
	"platinum": LicenseTypePlatinum,
}

func (l LicenseType) MarshalJSON() ([]byte, error) {
	buffer := bytes.NewBufferString(`"`)
	buffer.WriteString(l.String())
	buffer.WriteString(`"`)
	return buffer.Bytes(), nil
}

func (s *LicenseType) UnmarshalJSON(b []byte) error {
	var j string
	err := json.Unmarshal(b, &j)
	if err != nil {
		return err
	}
	*s = LicenseTypeFromString[j]
	return nil
}

func (l LicenseType) String() string {
	return licenseTypeToString[l]
}

// ClusterLicenseSpec defines the desired state of ClusterLicense
type ClusterLicenseSpec struct {
	// UID is the license UID not the k8s API UID (!)
	UID                string                 `json:"uid"`
	Type               LicenseType            `json:"type,string"`
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

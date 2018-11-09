package elasticsearch

type TrustRootConfig struct {
	Trust TrustConfig `json:"trust,omitempty"`
}

type TrustConfig struct {
	SubjectName []string `json:"subject_name,omitempty"`
}

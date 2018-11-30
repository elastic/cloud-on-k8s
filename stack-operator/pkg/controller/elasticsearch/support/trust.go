package support

// TrustRootConfig is the root of an Elasticsearch trust restrictions file
type TrustRootConfig struct {
	// Trust contains configuration for the Elasticsearch trust restrictions
	Trust TrustConfig `json:"trust,omitempty"`
}

// TrustConfig contains configuration for the Elasticsearch trust restrictions
type TrustConfig struct {
	// SubjectName is a list of patterns that incoming TLS client certificates must match
	SubjectName []string `json:"subject_name,omitempty"`
}

package common

import "fmt"

// MonitoringConfig contains the stack monitoring configuration for Beats.
type MonitoringConfig struct {
	Enabled       bool                `json:"enabled,omitempty"`
	Elasticsearch ElasticsearchConfig `json:"elasticsearch"`
}

// ElasticsearchConfig contains the configuration for connecting to Elasticsearch.
type ElasticsearchConfig struct {
	// Hosts are the Elasticsearch host urls to use.
	Hosts []string `json:"hosts"`
	// Username is the Elasticsearch username.
	Username string `json:"username"`
	// Password is the Elasticsearch password.
	Password string `json:"password"`
	// SSL is the ssl configuration for communicating with Elasticsearch.
	SSL SSLConfig `json:"ssl,omitempty"`
}

/// SSLConfig contains the SSL configuration for Beat stack monitoring.
type SSLConfig struct {
	// CertificateAuthorities contains a slice of filenames that contain PEM formatted certificate authorities.
	CertificateAuthorities []string `config:"certificate_authorities" yaml:"certificate_authorities"`
	// VerificationMode contains the verification mode for server certificates. Valid options: [full, strict, certificate, none]
	VerificationMode string `config:"verification_mode" yaml:"verification_mode"`
}

func getMonitoringCASecretName(beatName string) string {
	return fmt.Sprintf("%s-monitoring-ca", beatName)
}

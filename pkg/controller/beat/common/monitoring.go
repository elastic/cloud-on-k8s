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
	CertificateAuthorities []string `json:"certificate_authorities,omitempty"`
	VerificationMode       string   `json:"verification_mode,omitempty"`
}

func getMonitoringCASecretName(beatName string) string {
	return fmt.Sprintf("%s-monitoring-ca", beatName)
}

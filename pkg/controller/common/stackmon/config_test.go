// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackmon

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/name"
)

func TestBuildMetricbeatBaseConfig(t *testing.T) {
	tests := []struct {
		name       string
		isTLS      bool
		baseConfig string
	}{
		{
			name:  "with tls",
			isTLS: true,
			baseConfig: `
				hosts: ["scheme://localhost:1234"]
				username: elastic-internal-monitoring
				password: 1234567890
				ssl.certificate_authorities: ["/mnt/elastic-internal/xx-monitoring/namespace/name/certs/ca.crt"]
				ssl.verification_mode: "certificate"`,
		},
		{
			name:  "without tls",
			isTLS: false,
			baseConfig: `
				hosts: ["scheme://localhost:1234"]
				username: elastic-internal-monitoring
				password: 1234567890`,
		},
	}

	baseConfigTemplate := `
				hosts: ["{{ .URL }}"]
				username: {{ .Username }}
				password: {{ .Password }}
				{{- if .IsSSL }}
				ssl.certificate_authorities: ["{{ .SSLPath }}"]
				ssl.verification_mode: "{{ .SSLMode }}"
				{{- end }}`
	sampleURL := "scheme://localhost:1234"

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			baseConfig, _, err := buildMetricbeatBaseConfig(
				"xx-monitoring",
				types.NamespacedName{Namespace: "namespace", Name: "name"},
				name.NewNamer("xx"),
				sampleURL,
				"elastic-internal-monitoring",
				"1234567890",
				tc.isTLS,
				baseConfigTemplate,
			)
			assert.NoError(t, err)
			assert.Equal(t, tc.baseConfig, baseConfig)
		})
	}
}

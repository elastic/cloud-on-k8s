// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackmon

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func TestBuildMetricbeatBaseConfig(t *testing.T) {
	tests := []struct {
		name       string
		isCA       bool
		baseConfig string
	}{
		{
			name: "with tls",
			isCA: true,
			baseConfig: `
				hosts: ["scheme://localhost:1234"]
				username: elastic-internal-monitoring
				password: 1234567890
				ssl.certificate_authorities: ["/mnt/elastic-internal/xx-monitoring/namespace/name/certs/ca.crt"]
				ssl.verification_mode: "certificate"`,
		},
		{
			name: "without CA",
			isCA: false,
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
				{{- if .IsCA }}
				ssl.certificate_authorities: ["{{ .CAPath }}"]
				ssl.verification_mode: "{{ .SSLMode }}"
				{{- end }}`
	sampleURL := "scheme://localhost:1234"

	fakeClient := k8s.NewFakeClient(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "name-es-internal-users", Namespace: "namespace"},
		Data:       map[string][]byte{"elastic-internal-monitoring": []byte("1234567890")},
	})

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			baseConfig, _, err := buildMetricbeatBaseConfig(
				fakeClient,
				"xx-monitoring",
				types.NamespacedName{Namespace: "namespace", Name: "name"},
				types.NamespacedName{Namespace: "namespace", Name: "name"},
				name.NewNamer("xx"),
				sampleURL,
				tc.isCA,
				baseConfigTemplate,
			)
			assert.NoError(t, err)
			assert.Equal(t, tc.baseConfig, baseConfig)
		})
	}
}

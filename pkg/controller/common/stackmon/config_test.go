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
		isTLS      bool
		hasCA      bool
		baseConfig string
	}{
		{
			name:  "with TLS and a CA",
			isTLS: true,
			hasCA: true,
			baseConfig: `
				hosts: ["scheme://localhost:1234"]
				username: elastic-internal-monitoring
				password: 1234567890
				ssl.enabled: true
				ssl.verification_mode: "certificate"
				ssl.certificate_authorities: ["/mnt/elastic-internal/xx-monitoring/namespace/name/certs/ca.crt"]`,
		},
		{
			name:  "with TLS and no CA",
			isTLS: true,
			hasCA: false,
			baseConfig: `
				hosts: ["scheme://localhost:1234"]
				username: elastic-internal-monitoring
				password: 1234567890
				ssl.enabled: true
				ssl.verification_mode: "certificate"`,
		},
		{
			name:  "without TLS",
			isTLS: false,
			hasCA: false,
			baseConfig: `
				hosts: ["scheme://localhost:1234"]
				username: elastic-internal-monitoring
				password: 1234567890
				ssl.enabled: false
				ssl.verification_mode: "certificate"`,
		},
	}
	baseConfigTemplate := `
				hosts: ["{{ .URL }}"]
				username: {{ .Username }}
				password: {{ .Password }}
				ssl.enabled: {{ .IsSSL }}
				ssl.verification_mode: "certificate"
				{{- if .HasCA }}
				ssl.certificate_authorities: ["{{ .CAPath }}"]
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
				tc.isTLS,
				tc.hasCA,
				baseConfigTemplate,
			)
			assert.NoError(t, err)
			assert.Equal(t, tc.baseConfig, baseConfig)
		})
	}
}

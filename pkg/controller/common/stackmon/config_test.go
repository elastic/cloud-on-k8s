// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackmon

import (
	"context"
	"testing"

	"github.com/blang/semver/v4"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/stackmon/monitoring"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func Test_newBeatConfig(t *testing.T) {
	type args struct {
		initObjects []client.Object
		beatName    string
		baseConfig  string
		associated  commonv1.Associated
	}
	tests := []struct {
		name    string
		args    args
		want    beatConfig
		wantErr bool
	}{
		{
			name: "Simple output config",
			args: args{
				baseConfig: `
param1: value1
param2: value2
`,
				initObjects: []client.Object{
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "monitored-default-monitoring-beat-es-mon-user",
							Namespace: "default",
						},
						Data: map[string][]byte{
							"default-monitored-default-monitoring-beat-es-mon-user": []byte("password"),
						},
					},
				},
				beatName: "metricbeat",
				associated: &esv1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "monitored",
						Namespace: "default",
						Annotations: map[string]string{
							commonv1.ElasticsearchConfigAnnotationName(commonv1.ObjectSelector{Name: "monitoring", Namespace: "default"}): `
{
	"authSecretName": "monitored-default-monitoring-beat-es-mon-user",
	"authSecretKey": "default-monitored-default-monitoring-beat-es-mon-user",
	"isServiceAccount": false,
	"caCertProvided": true,
	"caSecretName": "monitored-es-monitoring-default-monitoring-ca",
	"url": "https://monitoring-es-http.default.svc:9200",
	"version": "8.4.0"
}
`,
						},
					},
					Spec: esv1.ElasticsearchSpec{
						Monitoring: commonv1.Monitoring{
							Metrics: commonv1.MetricsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "monitoring"}}},
						},
					},
				},
			},
			want: beatConfig{
				secret: corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name: "monitored-es-monitoring-metricbeat-config",
					},
					Data: map[string][]byte{
						"metricbeat.yml": []byte(`output:
    elasticsearch:
        hosts:
            - https://monitoring-es-http.default.svc:9200
        password: password
        ssl:
            certificate_authorities:
                - /mnt/elastic-internal/es-monitoring-association/default/monitoring/certs/ca.crt
            verification_mode: certificate
        username: default-monitored-default-monitoring-beat-es-mon-user
param1: value1
param2: value2
`),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := k8s.NewFakeClient(tt.args.initObjects...)
			got, err := newBeatConfig(
				context.Background(),
				fakeClient,
				tt.args.beatName,
				tt.args.associated.(monitoring.HasMonitoring),
				tt.args.associated.GetAssociations(),
				tt.args.baseConfig,
			)
			if (err != nil) != tt.wantErr {
				t.Errorf("newBeatConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			// Compare Beat configuration
			assert.Equal(t, tt.want.secret.Name, got.secret.Name)
			assert.Equal(t, tt.want.secret.Data, got.secret.Data)
		})
	}
}

func TestBuildMetricbeatBaseConfig(t *testing.T) {
	tests := []struct {
		name        string
		isTLS       bool
		certsSecret *corev1.Secret
		hasCA       bool
		baseConfig  string
		version     semver.Version
	}{
		{
			name:  "with TLS and a CA",
			isTLS: true,
			certsSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "name-es-http-certs-public", Namespace: "namespace"},
				Data: map[string][]byte{
					"tls.crt": []byte("1234567890"),
					"ca.crt":  []byte("1234567890"),
				},
			},
			baseConfig: `
				hosts: ["scheme://localhost:1234"]
				username: elastic-internal-monitoring
				password: 1234567890
				ssl.enabled: true
				ssl.verification_mode: "certificate"
				ingest_pipeline: "enabled"
				ssl.certificate_authorities: ["/mnt/elastic-internal/xx-monitoring/namespace/name/certs/ca.crt"]`,
			version: semver.MustParse("8.7.0"),
		},
		{
			name:  "with TLS and no CA",
			isTLS: true,
			certsSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "name-es-http-certs-public", Namespace: "namespace"},
				Data: map[string][]byte{
					"tls.crt": []byte("1234567890"),
				},
			},
			baseConfig: `
				hosts: ["scheme://localhost:1234"]
				username: elastic-internal-monitoring
				password: 1234567890
				ssl.enabled: true
				ssl.verification_mode: "certificate"
				ingest_pipeline: "enabled"`,
			version: semver.MustParse("8.7.0"),
		},
		{
			name:  "without TLS",
			isTLS: false,
			baseConfig: `
				hosts: ["scheme://localhost:1234"]
				username: elastic-internal-monitoring
				password: 1234567890
				ssl.enabled: false
				ssl.verification_mode: "certificate"
				ingest_pipeline: "enabled"`,
			version: semver.MustParse("8.7.0"),
		},
		{
			name:  "with version less than 8.7.0",
			isTLS: false,
			baseConfig: `
				hosts: ["scheme://localhost:1234"]
				username: elastic-internal-monitoring
				password: 1234567890
				ssl.enabled: false
				ssl.verification_mode: "certificate"`,
			version: semver.MustParse("8.6.0"),
		},
	}
	baseConfigTemplate := `
				hosts: ["{{ .URL }}"]
				username: {{ .Username }}
				password: {{ .Password }}
				ssl.enabled: {{ .IsSSL }}
				ssl.verification_mode: "certificate"
				{{- if isVersionGTE "8.7.0" }}
				ingest_pipeline: "enabled"
				{{- end }}
				{{- if .HasCA }}
				ssl.certificate_authorities: ["{{ .CAPath }}"]
				{{- end }}`

	sampleURL := "scheme://localhost:1234"
	internalUsersSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "name-es-internal-users", Namespace: "namespace"},
		Data:       map[string][]byte{"elastic-internal-monitoring": []byte("1234567890")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			initObjects := []client.Object{internalUsersSecret}
			if tc.certsSecret != nil {
				initObjects = append(initObjects, tc.certsSecret)
			}
			fakeClient := k8s.NewFakeClient(initObjects...)
			baseConfig, _, err := buildMetricbeatBaseConfig(
				fakeClient,
				"xx-monitoring",
				types.NamespacedName{Namespace: "namespace", Name: "name"},
				name.NewNamer("es"),
				sampleURL,
				"elastic-internal-monitoring",
				"1234567890",
				tc.isTLS,
				baseConfigTemplate,
				tc.version,
			)
			assert.NoError(t, err)
			assert.Equal(t, tc.baseConfig, baseConfig)
		})
	}
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackmon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/stackmon/monitoring"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
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
			hasMonitoring, ok := tt.args.associated.(monitoring.HasMonitoring)
			if !ok {
				t.Fatalf("associated is expected to implement monitoring.HasMonitoring")
			}
			got, err := newBeatConfig(
				context.Background(),
				fakeClient,
				tt.args.beatName,
				hasMonitoring,
				tt.args.associated.GetAssociations(),
				tt.args.baseConfig,
				metadata.Metadata{},
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

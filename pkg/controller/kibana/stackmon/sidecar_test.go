// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackmon

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gkampitakis/go-snaps/snaps"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/stackmon/monitoring"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

var (
	esRef    = commonv1.ObjectSelector{Name: "sample", Namespace: "aerospace"}
	sampleKb = kbv1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sample",
			Namespace: "aerospace",
		},
		Spec: kbv1.KibanaSpec{
			Version:          "7.14.0",
			ElasticsearchRef: esRef,
		},
	}
	kbFixtureWithMetricsMonitoring = func(kb kbv1.Kibana, esRef []commonv1.ObjectSelector, conf commonv1.AssociationConf) kbv1.Kibana {
		kb.Spec.Monitoring.Metrics.ElasticsearchRefs = esRef
		monitoring.GetMetricsAssociation(&kb)[0].SetAssociationConf(&conf)
		return kb
	}
	kbFixtureWithLogsMonitoring = func(kb kbv1.Kibana, esRef []commonv1.ObjectSelector, conf commonv1.AssociationConf) kbv1.Kibana {
		kb.Spec.Monitoring.Logs.ElasticsearchRefs = esRef
		monitoring.GetLogsAssociation(&kb)[0].SetAssociationConf(&conf)
		return kb
	}
	monitoringEsRef       = []commonv1.ObjectSelector{{Name: "monitoring", Namespace: "observability"}}
	logsEsRef             = []commonv1.ObjectSelector{{Name: "logs", Namespace: "observability"}}
	fakeElasticUserSecret = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "sample-es-internal-users", Namespace: "aerospace"},
		Data:       map[string][]byte{"elastic-internal-monitoring": []byte("1234567890")},
	}
	fakeMetricsBeatUserSecret = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "sample-observability-monitoring-beat-es-mon-user", Namespace: "aerospace"},
		Data:       map[string][]byte{"aerospace-sample-observability-monitoring-beat-es-mon-user": []byte("1234567890")},
	}
	fakeLogsBeatUserSecret = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "sample-observability-logs-beat-es-mon-user", Namespace: "aerospace"},
		Data:       map[string][]byte{"aerospace-sample-observability-logs-beat-es-mon-user": []byte("1234567890")},
	}
	fakeEsHTTPCertSecret = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "sample-es-http-certs-public", Namespace: "aerospace"},
		Data: map[string][]byte{
			"tls.crt": []byte("7H1515N074r341C3r71F1C473"),
			"ca.crt":  []byte("7H1515N074r341C3r71F1C473"),
		},
	}
	fakeKbHTTPCertSecret = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "sample-kb-http-certs-public", Namespace: "aerospace"},
		Data: map[string][]byte{
			"tls.crt": []byte("7H1515N074r341C3r71F1C473"),
			"ca.crt":  []byte("7H1515N074r341C3r71F1C473"),
		},
	}
	fakeClient = k8s.NewFakeClient(&fakeElasticUserSecret, &fakeMetricsBeatUserSecret, &fakeLogsBeatUserSecret, &fakeEsHTTPCertSecret, &fakeKbHTTPCertSecret)

	monitoringAssocConf = commonv1.AssociationConf{
		AuthSecretName: "sample-observability-monitoring-beat-es-mon-user",
		AuthSecretKey:  "aerospace-sample-observability-monitoring-beat-es-mon-user",
		CACertProvided: true,
		CASecretName:   "sample-es-monitoring-observability-monitoring-ca",
		URL:            "https://monitoring-es-http.observability.svc:9200",
		Version:        "7.14.0",
	}
	logsAssocConf = commonv1.AssociationConf{
		AuthSecretName: "sample-observability-logs-beat-es-mon-user",
		AuthSecretKey:  "aerospace-sample-observability-logs-beat-es-mon-user",
		CACertProvided: true,
		CASecretName:   "sample-es-logs-observability-monitoring-ca",
		URL:            "https://logs-es-http.observability.svc:9200",
		Version:        "7.14.0",
	}
)

func TestWithMonitoring(t *testing.T) {
	tests := []struct {
		name                   string
		kb                     func() kbv1.Kibana
		readOnlyRootFilesystem bool
	}{
		{
			name: "without monitoring",
			kb: func() kbv1.Kibana {
				return sampleKb
			},
			readOnlyRootFilesystem: false,
		},
		{
			name: "with metrics monitoring",
			kb: func() kbv1.Kibana {
				return kbFixtureWithMetricsMonitoring(sampleKb, monitoringEsRef, monitoringAssocConf)
			},
			readOnlyRootFilesystem: false,
		},
		{
			name: "with logs monitoring",
			kb: func() kbv1.Kibana {
				return kbFixtureWithLogsMonitoring(sampleKb, monitoringEsRef, monitoringAssocConf)
			},
			readOnlyRootFilesystem: false,
		},
		{
			name: "with metrics and logs monitoring",
			kb: func() kbv1.Kibana {
				return kbFixtureWithLogsMonitoring(
					kbFixtureWithMetricsMonitoring(
						sampleKb,
						monitoringEsRef,
						monitoringAssocConf),
					monitoringEsRef,
					logsAssocConf,
				)
			},
			readOnlyRootFilesystem: false,
		},
		{
			name: "with metrics and logs monitoring with different es ref",
			kb: func() kbv1.Kibana {
				return kbFixtureWithLogsMonitoring(
					kbFixtureWithMetricsMonitoring(
						sampleKb,
						monitoringEsRef,
						monitoringAssocConf),
					logsEsRef,
					logsAssocConf,
				)
			},
			readOnlyRootFilesystem: false,
		},
		{
			name: "with metrics and logs monitoring and read only root filesystem",
			kb: func() kbv1.Kibana {
				return kbFixtureWithLogsMonitoring(
					kbFixtureWithMetricsMonitoring(
						sampleKb,
						monitoringEsRef,
						monitoringAssocConf),
					logsEsRef,
					logsAssocConf,
				)
			},
			readOnlyRootFilesystem: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			kb := tc.kb()
			builder := defaults.NewPodTemplateBuilder(corev1.PodTemplateSpec{}, kbv1.KibanaContainerName)
			_, err := WithMonitoring(context.Background(), fakeClient, builder, kb, "", tc.readOnlyRootFilesystem)
			assert.NoError(t, err)

			actual, err := json.MarshalIndent(builder.PodTemplate, " ", "")
			assert.NoError(t, err)
			snaps.MatchJSON(t, actual)
		})
	}
}

func TestMetricbeatConfig(t *testing.T) {
	type args struct {
		client   k8s.Client
		kb       kbv1.Kibana
		basePath string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "no monitoring",
			args: args{
				client: k8s.NewFakeClient(),
				kb:     sampleKb,
			},
			wantErr: true,
		},
		{
			name: "with metrics monitoring",
			args: args{
				client: fakeClient,
				kb:     kbFixtureWithMetricsMonitoring(sampleKb, monitoringEsRef, monitoringAssocConf),
			},
			wantErr: false,
		},
		{
			name: "with monitoring no CA",
			args: args{
				client: k8s.NewFakeClient(&fakeElasticUserSecret, &fakeMetricsBeatUserSecret, &fakeLogsBeatUserSecret, &fakeEsHTTPCertSecret, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "sample-kb-http-certs-public", Namespace: "aerospace"},
					Data: map[string][]byte{
						"tls.crt": []byte("7H1515N074r341C3r71F1C473"),
					},
				}),
				kb: kbFixtureWithMetricsMonitoring(sampleKb, monitoringEsRef, monitoringAssocConf),
			},
			wantErr: false,
		},
		{
			name: "with metrics monitoring no TLS",
			args: args{
				client: fakeClient,
				kb: func() kbv1.Kibana {
					kb := kbFixtureWithMetricsMonitoring(sampleKb, monitoringEsRef, monitoringAssocConf)
					kb.Spec.HTTP.TLS = commonv1.TLSOptions{
						SelfSignedCertificate: &commonv1.SelfSignedCertificate{Disabled: true},
					}
					return kb
				}(),
			},
			wantErr: false,
		},
		{
			name: "with metrics monitoring and basePath",
			args: args{
				client:   fakeClient,
				kb:       kbFixtureWithMetricsMonitoring(sampleKb, monitoringEsRef, monitoringAssocConf),
				basePath: "/prefix",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Metricbeat(context.Background(), tt.args.client, tt.args.kb, tt.args.basePath)
			if (err != nil) != tt.wantErr {
				t.Errorf("Metricbeat() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			snaps.MatchSnapshot(t, string(got.ConfigSecret.Data["metricbeat.yml"]))
		})
	}
}

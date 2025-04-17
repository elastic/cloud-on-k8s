// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackmon

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/gkampitakis/go-snaps/snaps"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/stackmon"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/stackmon/monitoring"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/logstash/configs"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func TestWithMonitoring(t *testing.T) {
	sampleLs := logstashv1alpha1.Logstash{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sample",
			Namespace: "aerospace",
		},
		Spec: logstashv1alpha1.LogstashSpec{
			Version: "8.6.0",
		},
	}
	monitoringEsRef := []commonv1.ObjectSelector{{Name: "monitoring", Namespace: "observability"}}
	logsEsRef := []commonv1.ObjectSelector{{Name: "logs", Namespace: "observability"}}

	fakeMetricsBeatUserSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "sample-observability-monitoring-beat-es-mon-user", Namespace: "aerospace"},
		Data:       map[string][]byte{"aerospace-sample-observability-monitoring-beat-es-mon-user": []byte("1234567890")},
	}
	fakeLogsBeatUserSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "sample-observability-logs-beat-es-mon-user", Namespace: "aerospace"},
		Data:       map[string][]byte{"aerospace-sample-observability-logs-beat-es-mon-user": []byte("1234567890")},
	}
	fakeEsHTTPCertSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "sample-es-http-certs-public", Namespace: "aerospace"},
		Data: map[string][]byte{
			"tls.crt": []byte("7H1515N074r341C3r71F1C473"),
			"ca.crt":  []byte("7H1515N074r341C3r71F1C473"),
		},
	}
	fakeLsHTTPCertSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "sample-ls-http-certs-public", Namespace: "aerospace"},
		Data: map[string][]byte{
			"tls.crt": []byte("7H1515N074r341C3r71F1C473"),
			"ca.crt":  []byte("7H1515N074r341C3r71F1C473"),
		},
	}
	fakeClient := k8s.NewFakeClient(&fakeMetricsBeatUserSecret, &fakeLogsBeatUserSecret, &fakeEsHTTPCertSecret, &fakeLsHTTPCertSecret)

	monitoringAssocConf := commonv1.AssociationConf{
		AuthSecretName: "sample-observability-monitoring-beat-es-mon-user",
		AuthSecretKey:  "aerospace-sample-observability-monitoring-beat-es-mon-user",
		CACertProvided: true,
		CASecretName:   "sample-es-monitoring-observability-monitoring-ca",
		URL:            "https://monitoring-es-http.observability.svc:9200",
		Version:        "8.6.0",
	}
	logsAssocConf := commonv1.AssociationConf{
		AuthSecretName: "sample-observability-logs-beat-es-mon-user",
		AuthSecretKey:  "aerospace-sample-observability-logs-beat-es-mon-user",
		CACertProvided: true,
		CASecretName:   "sample-es-logs-observability-monitoring-ca",
		URL:            "https://logs-es-http.observability.svc:9200",
		Version:        "8.6.0",
	}

	tests := []struct {
		name            string
		ls              func() logstashv1alpha1.Logstash
		apiServerConfig configs.APIServer
	}{
		{
			name: "without monitoring",
			ls: func() logstashv1alpha1.Logstash {
				return sampleLs
			},
		},
		{
			name: "with metrics monitoring",
			ls: func() logstashv1alpha1.Logstash {
				sampleLs.Spec.Monitoring.Metrics.ElasticsearchRefs = monitoringEsRef
				monitoring.GetMetricsAssociation(&sampleLs)[0].SetAssociationConf(&monitoringAssocConf)
				return sampleLs
			},
			apiServerConfig: GetAPIServerWithSSLEnabled(false),
		},
		{
			name: "with TLS metrics monitoring",
			ls: func() logstashv1alpha1.Logstash {
				sampleLs.Spec.Monitoring.Metrics.ElasticsearchRefs = monitoringEsRef
				monitoring.GetMetricsAssociation(&sampleLs)[0].SetAssociationConf(&monitoringAssocConf)
				return sampleLs
			},
			apiServerConfig: GetAPIServerWithSSLEnabled(true),
		},
		{
			name: "with logs monitoring",
			ls: func() logstashv1alpha1.Logstash {
				sampleLs.Spec.Monitoring.Metrics.ElasticsearchRefs = nil
				sampleLs.Spec.Monitoring.Logs.ElasticsearchRefs = monitoringEsRef
				monitoring.GetLogsAssociation(&sampleLs)[0].SetAssociationConf(&monitoringAssocConf)
				return sampleLs
			},
			apiServerConfig: GetAPIServerWithSSLEnabled(false),
		},
		{
			name: "with metrics and logs monitoring",
			ls: func() logstashv1alpha1.Logstash {
				sampleLs.Spec.Monitoring.Metrics.ElasticsearchRefs = monitoringEsRef
				monitoring.GetMetricsAssociation(&sampleLs)[0].SetAssociationConf(&monitoringAssocConf)
				sampleLs.Spec.Monitoring.Logs.ElasticsearchRefs = monitoringEsRef
				monitoring.GetLogsAssociation(&sampleLs)[0].SetAssociationConf(&logsAssocConf)
				return sampleLs
			},
			apiServerConfig: GetAPIServerWithSSLEnabled(false),
		},
		{
			name: "with metrics and logs monitoring with different es ref",
			ls: func() logstashv1alpha1.Logstash {
				sampleLs.Spec.Monitoring.Metrics.ElasticsearchRefs = monitoringEsRef
				monitoring.GetMetricsAssociation(&sampleLs)[0].SetAssociationConf(&monitoringAssocConf)
				sampleLs.Spec.Monitoring.Logs.ElasticsearchRefs = logsEsRef
				monitoring.GetLogsAssociation(&sampleLs)[0].SetAssociationConf(&logsAssocConf)
				return sampleLs
			},
			apiServerConfig: GetAPIServerWithSSLEnabled(false),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ls := tc.ls()
			builder := defaults.NewPodTemplateBuilder(corev1.PodTemplateSpec{}, logstashv1alpha1.LogstashContainerName)
			_, err := WithMonitoring(context.Background(), fakeClient, builder, ls, tc.apiServerConfig)
			assert.NoError(t, err)
			actual, err := json.MarshalIndent(builder.PodTemplate, " ", "")
			assert.NoError(t, err)
			snaps.MatchJSON(t, actual)
		})
	}
}

func GetAPIServerWithSSLEnabled(enabled bool) configs.APIServer {
	return configs.APIServer{
		SSLEnabled:       strconv.FormatBool(enabled),
		KeystorePassword: "blablabla",
		AuthType:         "basic",
		Username:         "logstash",
		Password:         "whatever",
	}
}

func TestMetricbeatConfig(t *testing.T) {
	volumeFixture := volume.NewSecretVolumeWithMountPath(
		"secret-name",
		"ls-ca",
		"/mount",
	)
	type args struct {
		URL      string
		Username string
		Password string
		IsSSL    bool
		CAVolume volume.VolumeLike
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "default",
			args: args{
				URL:      "https://localhost:9200",
				Username: "elastic",
				Password: "password",
				IsSSL:    true,
				CAVolume: volumeFixture,
			},
		},
		{
			name: "no password",
			args: args{
				URL:      "https://localhost:9200",
				Username: "elastic",
				Password: "",
				IsSSL:    true,
				CAVolume: volumeFixture,
			},
		},
		{
			name: "no user + password",
			args: args{
				URL:      "https://localhost:9200",
				Username: "",
				Password: "",
				IsSSL:    true,
				CAVolume: volumeFixture,
			},
		},
		{
			name: "no TLS",
			args: args{
				URL:      "https://localhost:9200",
				Username: "elastic",
				Password: "password",
				IsSSL:    false,
				CAVolume: volumeFixture,
			},
		},
		{
			name: "no CA",
			args: args{
				URL:      "https://localhost:9200",
				Username: "elastic",
				Password: "password",
				IsSSL:    true,
				CAVolume: nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := stackmon.RenderTemplate(version.From(8, 16, 0), metricbeatConfigTemplate, tt.args)
			require.NoError(t, err)
			snaps.MatchSnapshot(t, cfg)
		})
	}
}

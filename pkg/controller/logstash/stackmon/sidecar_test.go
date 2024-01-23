// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackmon

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/stackmon/monitoring"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/configs"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
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
		name                      string
		ls                        func() logstashv1alpha1.Logstash
		apiServerConfig           configs.APIServer
		containersLength          int
		esEnvVarsLength           int
		podVolumesLength          int
		metricsVolumeMountsLength int
		logVolumeMountsLength     int
	}{
		{
			name: "without monitoring",
			ls: func() logstashv1alpha1.Logstash {
				return sampleLs
			},
			containersLength: 1,
		},
		{
			name: "with metrics monitoring",
			ls: func() logstashv1alpha1.Logstash {
				sampleLs.Spec.Monitoring.Metrics.ElasticsearchRefs = monitoringEsRef
				monitoring.GetMetricsAssociation(&sampleLs)[0].SetAssociationConf(&monitoringAssocConf)
				return sampleLs
			},
			apiServerConfig:           GetAPIServerWithSSLEnabled(false),
			containersLength:          2,
			esEnvVarsLength:           0,
			podVolumesLength:          3,
			metricsVolumeMountsLength: 3,
		},
		{
			name: "with TLS metrics monitoring",
			ls: func() logstashv1alpha1.Logstash {
				sampleLs.Spec.Monitoring.Metrics.ElasticsearchRefs = monitoringEsRef
				monitoring.GetMetricsAssociation(&sampleLs)[0].SetAssociationConf(&monitoringAssocConf)
				return sampleLs
			},
			apiServerConfig:           GetAPIServerWithSSLEnabled(true),
			containersLength:          2,
			esEnvVarsLength:           0,
			podVolumesLength:          4,
			metricsVolumeMountsLength: 4,
		},
		{
			name: "with logs monitoring",
			ls: func() logstashv1alpha1.Logstash {
				sampleLs.Spec.Monitoring.Metrics.ElasticsearchRefs = nil
				sampleLs.Spec.Monitoring.Logs.ElasticsearchRefs = monitoringEsRef
				monitoring.GetLogsAssociation(&sampleLs)[0].SetAssociationConf(&monitoringAssocConf)
				return sampleLs
			},
			apiServerConfig:       GetAPIServerWithSSLEnabled(false),
			containersLength:      2,
			esEnvVarsLength:       1,
			podVolumesLength:      3,
			logVolumeMountsLength: 4,
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
			apiServerConfig:           GetAPIServerWithSSLEnabled(false),
			containersLength:          3,
			esEnvVarsLength:           1,
			podVolumesLength:          5,
			metricsVolumeMountsLength: 3,
			logVolumeMountsLength:     4,
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
			apiServerConfig:           GetAPIServerWithSSLEnabled(false),
			containersLength:          3,
			esEnvVarsLength:           1,
			podVolumesLength:          6,
			metricsVolumeMountsLength: 3,
			logVolumeMountsLength:     4,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ls := tc.ls()
			builder := defaults.NewPodTemplateBuilder(corev1.PodTemplateSpec{}, logstashv1alpha1.LogstashContainerName)
			_, err := WithMonitoring(context.Background(), fakeClient, builder, ls, tc.apiServerConfig)
			assert.NoError(t, err)

			assert.Equal(t, tc.containersLength, len(builder.PodTemplate.Spec.Containers))
			for _, v := range builder.PodTemplate.Spec.Volumes {
				fmt.Println(v)
			}
			assert.Equal(t, tc.podVolumesLength, len(builder.PodTemplate.Spec.Volumes))

			if monitoring.IsMetricsDefined(&ls) {
				for _, c := range builder.PodTemplate.Spec.Containers {
					if c.Name == "metricbeat" {
						assert.Equal(t, tc.metricsVolumeMountsLength, len(c.VolumeMounts))
					}
				}
			}
			if monitoring.IsLogsDefined(&ls) {
				for _, c := range builder.PodTemplate.Spec.Containers {
					if c.Name == "filebeat" {
						assert.Equal(t, tc.logVolumeMountsLength, len(c.VolumeMounts))
					}
				}
			}
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

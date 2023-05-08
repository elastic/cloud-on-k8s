// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackmon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/stackmon/monitoring"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func TestWithMonitoring(t *testing.T) {
	sampleEs := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sample",
			Namespace: "aerospace",
		},
		Spec: esv1.ElasticsearchSpec{
			Version: "7.14.0",
		},
	}
	monitoringEsRef := []commonv1.ObjectSelector{{Name: "monitoring", Namespace: "observability"}}
	logsEsRef := []commonv1.ObjectSelector{{Name: "logs", Namespace: "observability"}}

	fakeElasticUserSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "sample-es-internal-users", Namespace: "aerospace"},
		Data:       map[string][]byte{"elastic-internal-monitoring": []byte("1234567890")},
	}
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
		Data:       map[string][]byte{"ca.crt": []byte("7H1515N074r341C3r71F1C473")},
	}
	fakeClient := k8s.NewFakeClient(&fakeElasticUserSecret, &fakeMetricsBeatUserSecret, &fakeLogsBeatUserSecret, &fakeEsHTTPCertSecret)

	monitoringAssocConf := commonv1.AssociationConf{
		AuthSecretName: "sample-observability-monitoring-beat-es-mon-user",
		AuthSecretKey:  "aerospace-sample-observability-monitoring-beat-es-mon-user",
		CACertProvided: true,
		CASecretName:   "sample-es-monitoring-observability-monitoring-ca",
		URL:            "https://monitoring-es-http.observability.svc:9200",
		Version:        "7.14.0",
	}
	logsAssocConf := commonv1.AssociationConf{
		AuthSecretName: "sample-observability-logs-beat-es-mon-user",
		AuthSecretKey:  "aerospace-sample-observability-logs-beat-es-mon-user",
		CACertProvided: true,
		CASecretName:   "sample-es-logs-observability-monitoring-ca",
		URL:            "https://logs-es-http.observability.svc:9200",
		Version:        "7.14.0",
	}

	tests := []struct {
		name                   string
		es                     func() esv1.Elasticsearch
		containersLength       int
		esEnvVarsLength        int
		podVolumesLength       int
		beatVolumeMountsLength int
	}{
		{
			name: "without monitoring",
			es: func() esv1.Elasticsearch {
				return sampleEs
			},
			containersLength: 1,
		},
		{
			name: "with metrics monitoring",
			es: func() esv1.Elasticsearch {
				sampleEs.Spec.Monitoring.Metrics.ElasticsearchRefs = monitoringEsRef
				monitoring.GetMetricsAssociation(&sampleEs)[0].SetAssociationConf(&monitoringAssocConf)
				return sampleEs
			},
			containersLength:       2,
			esEnvVarsLength:        0,
			podVolumesLength:       4,
			beatVolumeMountsLength: 4,
		},
		{
			name: "with logs monitoring",
			es: func() esv1.Elasticsearch {
				sampleEs.Spec.Monitoring.Metrics.ElasticsearchRefs = nil
				sampleEs.Spec.Monitoring.Logs.ElasticsearchRefs = monitoringEsRef
				monitoring.GetLogsAssociation(&sampleEs)[0].SetAssociationConf(&monitoringAssocConf)
				return sampleEs
			},
			containersLength:       2,
			esEnvVarsLength:        1,
			podVolumesLength:       3,
			beatVolumeMountsLength: 4,
		},
		{
			name: "with metrics and logs monitoring",
			es: func() esv1.Elasticsearch {
				sampleEs.Spec.Monitoring.Metrics.ElasticsearchRefs = monitoringEsRef
				monitoring.GetMetricsAssociation(&sampleEs)[0].SetAssociationConf(&monitoringAssocConf)
				sampleEs.Spec.Monitoring.Logs.ElasticsearchRefs = monitoringEsRef
				monitoring.GetLogsAssociation(&sampleEs)[0].SetAssociationConf(&logsAssocConf)
				return sampleEs
			},
			containersLength:       3,
			esEnvVarsLength:        1,
			podVolumesLength:       6,
			beatVolumeMountsLength: 4,
		},
		{
			name: "with metrics and logs monitoring with different es ref",
			es: func() esv1.Elasticsearch {
				sampleEs.Spec.Monitoring.Metrics.ElasticsearchRefs = monitoringEsRef
				monitoring.GetMetricsAssociation(&sampleEs)[0].SetAssociationConf(&monitoringAssocConf)
				sampleEs.Spec.Monitoring.Logs.ElasticsearchRefs = logsEsRef
				monitoring.GetLogsAssociation(&sampleEs)[0].SetAssociationConf(&logsAssocConf)
				return sampleEs
			},
			containersLength:       3,
			esEnvVarsLength:        1,
			podVolumesLength:       7,
			beatVolumeMountsLength: 4,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			es := tc.es()
			builder := defaults.NewPodTemplateBuilder(corev1.PodTemplateSpec{}, esv1.ElasticsearchContainerName)
			_, err := WithMonitoring(context.Background(), fakeClient, builder, es)
			assert.NoError(t, err)

			assert.Equal(t, tc.containersLength, len(builder.PodTemplate.Spec.Containers))
			assert.Equal(t, tc.esEnvVarsLength, len(builder.PodTemplate.Spec.Containers[0].Env))
			assert.Equal(t, tc.podVolumesLength, len(builder.PodTemplate.Spec.Volumes))

			if monitoring.IsMetricsDefined(&es) {
				for _, c := range builder.PodTemplate.Spec.Containers {
					if c.Name == "metricbeat" {
						assert.Equal(t, tc.beatVolumeMountsLength, len(c.VolumeMounts))
						assertSecurityContext(t, c.SecurityContext)
					}
				}
			}
			if monitoring.IsLogsDefined(&es) {
				for _, c := range builder.PodTemplate.Spec.Containers {
					if c.Name == "filebeat" {
						assert.Equal(t, tc.beatVolumeMountsLength, len(c.VolumeMounts))
						assertSecurityContext(t, c.SecurityContext)
					}
				}
			}
		})
	}
}

func assertSecurityContext(t *testing.T, securityContext *corev1.SecurityContext) {
	t.Helper()
	require.NotNil(t, securityContext)
	require.NotNil(t, securityContext.Privileged)
	require.False(t, *securityContext.Privileged)
	require.NotNil(t, securityContext.Capabilities)
	droppedCapabilities := securityContext.Capabilities.Drop
	hasDropAllCapability := false
	for _, capability := range droppedCapabilities {
		if capability == "ALL" {
			hasDropAllCapability = true
			break
		}
	}
	require.True(t, hasDropAllCapability, "ALL capability not found in securityContext.Capabilities.Drop")
}

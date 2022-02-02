// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackmon

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/stackmon/monitoring"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func TestWithMonitoring(t *testing.T) {
	esRef := commonv1.ObjectSelector{Name: "sample", Namespace: "aerospace"}
	sampleKb := kbv1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sample",
			Namespace: "aerospace",
		},
		Spec: kbv1.KibanaSpec{
			Version:          "7.14.0",
			ElasticsearchRef: esRef,
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
		Data: map[string][]byte{
			"tls.crt": []byte("7H1515N074r341C3r71F1C473"),
			"ca.crt":  []byte("7H1515N074r341C3r71F1C473"),
		},
	}
	fakeKbHTTPCertSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "sample-kb-http-certs-public", Namespace: "aerospace"},
		Data: map[string][]byte{
			"tls.crt": []byte("7H1515N074r341C3r71F1C473"),
			"ca.crt":  []byte("7H1515N074r341C3r71F1C473"),
		},
	}
	fakeClient := k8s.NewFakeClient(&fakeElasticUserSecret, &fakeMetricsBeatUserSecret, &fakeLogsBeatUserSecret, &fakeEsHTTPCertSecret, &fakeKbHTTPCertSecret)

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
		kb                     func() kbv1.Kibana
		containersLength       int
		podVolumesLength       int
		beatVolumeMountsLength int
	}{
		{
			name: "without monitoring",
			kb: func() kbv1.Kibana {
				return sampleKb
			},
			containersLength: 1,
		},
		{
			name: "with metrics monitoring",
			kb: func() kbv1.Kibana {
				sampleKb.Spec.Monitoring.Metrics.ElasticsearchRefs = monitoringEsRef
				monitoring.GetMetricsAssociation(&sampleKb)[0].SetAssociationConf(&monitoringAssocConf)
				return sampleKb
			},
			containersLength:       2,
			podVolumesLength:       3,
			beatVolumeMountsLength: 3,
		},
		{
			name: "with logs monitoring",
			kb: func() kbv1.Kibana {
				sampleKb.Spec.Monitoring.Metrics.ElasticsearchRefs = nil
				sampleKb.Spec.Monitoring.Logs.ElasticsearchRefs = monitoringEsRef
				monitoring.GetLogsAssociation(&sampleKb)[0].SetAssociationConf(&monitoringAssocConf)
				return sampleKb
			},
			containersLength:       2,
			podVolumesLength:       3,
			beatVolumeMountsLength: 3,
		},
		{
			name: "with metrics and logs monitoring",
			kb: func() kbv1.Kibana {
				sampleKb.Spec.Monitoring.Metrics.ElasticsearchRefs = monitoringEsRef
				monitoring.GetMetricsAssociation(&sampleKb)[0].SetAssociationConf(&monitoringAssocConf)
				sampleKb.Spec.Monitoring.Logs.ElasticsearchRefs = monitoringEsRef
				monitoring.GetLogsAssociation(&sampleKb)[0].SetAssociationConf(&logsAssocConf)
				return sampleKb
			},
			containersLength:       3,
			podVolumesLength:       5,
			beatVolumeMountsLength: 3,
		},
		{
			name: "with metrics and logs monitoring with different es ref",
			kb: func() kbv1.Kibana {
				sampleKb.Spec.Monitoring.Metrics.ElasticsearchRefs = monitoringEsRef
				monitoring.GetMetricsAssociation(&sampleKb)[0].SetAssociationConf(&monitoringAssocConf)
				sampleKb.Spec.Monitoring.Logs.ElasticsearchRefs = logsEsRef
				monitoring.GetLogsAssociation(&sampleKb)[0].SetAssociationConf(&logsAssocConf)
				return sampleKb
			},
			containersLength:       3,
			podVolumesLength:       6,
			beatVolumeMountsLength: 3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			kb := tc.kb()
			builder := defaults.NewPodTemplateBuilder(corev1.PodTemplateSpec{}, kbv1.KibanaContainerName)
			_, err := WithMonitoring(fakeClient, builder, kb)
			assert.NoError(t, err)

			assert.Equal(t, tc.containersLength, len(builder.PodTemplate.Spec.Containers))
			for _, v := range builder.PodTemplate.Spec.Volumes {
				fmt.Println(v)
			}
			assert.Equal(t, tc.podVolumesLength, len(builder.PodTemplate.Spec.Volumes))

			if monitoring.IsMetricsDefined(&kb) {
				for _, c := range builder.PodTemplate.Spec.Containers {
					if c.Name == "metricbeat" {
						assert.Equal(t, tc.beatVolumeMountsLength, len(c.VolumeMounts))
					}
				}
			}
			if monitoring.IsLogsDefined(&kb) {
				for _, c := range builder.PodTemplate.Spec.Containers {
					if c.Name == "filebeat" {
						assert.Equal(t, tc.beatVolumeMountsLength, len(c.VolumeMounts))
					}
				}
			}
		})
	}
}

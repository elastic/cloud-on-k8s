// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackmon

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/blang/semver/v4"
	"github.com/gkampitakis/go-snaps/snaps"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/stackmon"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/stackmon/monitoring"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume"
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
		readOnlyRootFilesystem bool
	}{
		{
			name: "without monitoring",
			es: func() esv1.Elasticsearch {
				return sampleEs
			},
			readOnlyRootFilesystem: false,
		},
		{
			name: "with metrics monitoring",
			es: func() esv1.Elasticsearch {
				sampleEs.Spec.Monitoring.Metrics.ElasticsearchRefs = monitoringEsRef
				monitoring.GetMetricsAssociation(&sampleEs)[0].SetAssociationConf(&monitoringAssocConf)
				return sampleEs
			},
			readOnlyRootFilesystem: false,
		},
		{
			name: "with logs monitoring",
			es: func() esv1.Elasticsearch {
				sampleEs.Spec.Monitoring.Metrics.ElasticsearchRefs = nil
				sampleEs.Spec.Monitoring.Logs.ElasticsearchRefs = monitoringEsRef
				monitoring.GetLogsAssociation(&sampleEs)[0].SetAssociationConf(&monitoringAssocConf)
				return sampleEs
			},
			readOnlyRootFilesystem: false,
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
			readOnlyRootFilesystem: false,
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
			readOnlyRootFilesystem: false,
		},
		{
			name: "with metrics and logs monitoring and read only root filesystem",
			es: func() esv1.Elasticsearch {
				sampleEs.Spec.Monitoring.Metrics.ElasticsearchRefs = monitoringEsRef
				monitoring.GetMetricsAssociation(&sampleEs)[0].SetAssociationConf(&monitoringAssocConf)
				sampleEs.Spec.Monitoring.Logs.ElasticsearchRefs = monitoringEsRef
				monitoring.GetLogsAssociation(&sampleEs)[0].SetAssociationConf(&logsAssocConf)
				return sampleEs
			},
			readOnlyRootFilesystem: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			es := tc.es()
			builder := defaults.NewPodTemplateBuilder(corev1.PodTemplateSpec{}, esv1.ElasticsearchContainerName)
			_, err := WithMonitoring(context.Background(), fakeClient, builder, es, tc.readOnlyRootFilesystem)
			assert.NoError(t, err)

			actual, err := json.MarshalIndent(builder.PodTemplate, " ", "")
			assert.NoError(t, err)
			snaps.MatchJSON(t, actual)
		})
	}
}

func TestMetricbeatConfig(t *testing.T) {
	volumeFixture := volume.NewSecretVolumeWithMountPath(
		"secret-name",
		"es-ca",
		"/mount",
	)
	type args struct {
		URL      string
		Username string
		Password string
		IsSSL    bool
		CAVolume volume.VolumeLike
		Version  semver.Version
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
				Password: "secret",
				IsSSL:    true,
				Version:  version.From(8, 0, 0),
				CAVolume: volumeFixture,
			},
		},
		{
			name: "no CA",
			args: args{
				URL:      "https://localhost:9200",
				Username: "elastic",
				Password: "secret",
				IsSSL:    true,
				Version:  version.From(8, 0, 0),
			},
		},
		{
			name: "post 8.7.0 with ingest",
			args: args{
				URL:      "https://localhost:9200",
				Username: "elastic",
				Password: "secret",
				IsSSL:    true,
				Version:  version.From(8, 16, 0),
				CAVolume: volumeFixture,
			},
		},
		{
			name: "no TLS",
			args: args{
				URL:      "https://localhost:9200",
				Username: "elastic",
				Password: "secret",
				IsSSL:    false,
				Version:  version.From(8, 0, 0),
			},
		},
		{
			name: "latest ES versions: higher field limit",
			args: args{
				URL:      "https://localhost:9200",
				Username: "elastic",
				Password: "secret",
				IsSSL:    true,
				Version:  version.From(8, 17, 0),
				CAVolume: volumeFixture,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := stackmon.RenderTemplate(tt.args.Version, metricbeatConfigTemplate, tt.args)
			require.NoError(t, err)
			snaps.MatchSnapshot(t, cfg)
		})
	}
}

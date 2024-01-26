// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackmon

import (
	"context"
	_ "embed"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/stackmon"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/bootstrap"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func TestMetricBeat(t *testing.T) {
	containerFixture := corev1.Container{
		Name:  "metricbeat",
		Image: "docker.elastic.co/beats/metricbeat:8.2.3",
		Args:  []string{"-c", "/etc/metricbeat-config/metricbeat.yml", "-e"},
		Env: []corev1.EnvVar{
			{
				Name: "POD_IP",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						APIVersion: "v1",
						FieldPath:  "status.podIP",
					},
				},
			},
			{
				Name: "POD_NAME",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						APIVersion: "v1",
						FieldPath:  "metadata.name",
					},
				},
			},
			{
				Name: "NODE_NAME",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						APIVersion: "v1",
						FieldPath:  "spec.nodeName",
					},
				},
			},
			{
				Name: "NAMESPACE",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						APIVersion: "v1",
						FieldPath:  "metadata.namespace",
					},
				},
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "beat-metricbeat-config",
				ReadOnly:  true,
				MountPath: "/etc/metricbeat-config",
			},
			{
				Name:      "metricbeat-data",
				ReadOnly:  false,
				MountPath: "/usr/share/metricbeat/data",
			},
			{
				Name:      "shared-data",
				ReadOnly:  false,
				MountPath: "/var/shared",
			},
		},
	}
	beatYml := `http:
    enabled: false
metricbeat:
    modules:
        - hosts:
            - http+unix:///var/shared/metricbeat-test-beat.sock
          metricsets:
            - stats
            - state
          module: beat
          period: 10s
          xpack:
            enabled: true
monitoring:
    cluster_uuid: %s
    enabled: false
output:
    elasticsearch:
        hosts:
            - %s
        password: es-password
        ssl:
            verification_mode: certificate
        username: es-user
`
	beatSidecarFixture := func(beatYml string) stackmon.BeatSidecar {
		return stackmon.BeatSidecar{
			Container: containerFixture,
			ConfigSecret: corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "beat-beat-monitoring-metricbeat-config",
					Namespace: "test",
					Labels: map[string]string{
						"common.k8s.elastic.co/type": "beat",
						"beat.k8s.elastic.co/name":   "beat",
					},
				},
				Data: map[string][]byte{
					"metricbeat.yml": []byte(beatYml),
				},
			},
			Volumes: []corev1.Volume{
				{
					Name:         "beat-metricbeat-config",
					VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "beat-beat-monitoring-metricbeat-config", Optional: ptr.To[bool](false)}},
				},
				{
					Name:         "metricbeat-data",
					VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
				},
				{
					Name:         "shared-data",
					VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
				},
			},
		}
	}

	esFixture := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "es",
			Namespace:   "test",
			Annotations: map[string]string{bootstrap.ClusterUUIDAnnotationName: "abcd1234"},
		},
		Spec: esv1.ElasticsearchSpec{
			Version: "8.2.3",
		},
	}
	monitoringEsFixture := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "esmonitoring",
			Namespace:   "test",
			Annotations: map[string]string{bootstrap.ClusterUUIDAnnotationName: "abcd4321"},
		},
		Spec: esv1.ElasticsearchSpec{
			Version: "8.2.3",
		},
	}
	beatFixture := v1beta1.Beat{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "beat",
			Namespace: "test",
		},
		Spec: v1beta1.BeatSpec{
			Type:             "metricbeat",
			Version:          "8.2.3",
			ElasticsearchRef: commonv1.ObjectSelector{Name: "es", Namespace: "test"},
			Deployment:       &v1beta1.DeploymentSpec{},
			Config: &commonv1.Config{
				Data: map[string]interface{}{},
			},
			Monitoring: commonv1.Monitoring{
				Metrics: commonv1.MetricsMonitoring{
					ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "esmonitoring"}},
				},
				Logs: commonv1.LogsMonitoring{
					ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "esmonitoring"}},
				},
			},
		},
	}
	beatFixture.GetAssociations()[2].SetAssociationConf(&commonv1.AssociationConf{
		AuthSecretName: "es-secret-name",
		AuthSecretKey:  "es-user",
		URL:            "es-metrics-monitoring-url",
	})

	esAPIFixture := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.WriteHeader(200)
		_, _ = res.Write([]byte(`{
  "name": "instance-0000000000",
  "cluster_name": "86eed713483a440d8e5a0242e420726f",
  "cluster_uuid": "QGq3wcU7Sd6bC31wh37eKQ",
  "version": {
    "number": "8.6.2",
    "build_flavor": "default",
    "build_type": "docker",
    "build_hash": "2d58d0f136141f03239816a4e360a8d17b6d8f29",
    "build_date": "2023-02-13T09:35:20.314882762Z",
    "build_snapshot": false,
    "lucene_version": "9.4.2",
    "minimum_wire_compatibility_version": "7.17.0",
    "minimum_index_compatibility_version": "7.0.0"
  },
  "tagline": "You Know, for Search"
}`))
	}))
	defer esAPIFixture.Close()

	externalESSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "test", Name: "external-es"},
		Data: map[string][]byte{
			"url":      []byte(esAPIFixture.URL),
			"username": []byte("es-user"),
			"password": []byte("es-password"),
		},
	}

	type args struct {
		client  k8s.Client
		beat    func() *v1beta1.Beat
		version string
	}
	tests := []struct {
		name    string
		args    args
		want    stackmon.BeatSidecar
		wantErr bool
	}{
		{
			name: "beat with stack monitoring enabled and valid elasticsearchRef returns properly configured sidecar",
			args: args{
				client: k8s.NewFakeClient(&beatFixture, &esFixture, &monitoringEsFixture, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "es-secret-name", Namespace: "test"},
					Data:       map[string][]byte{"es-user": []byte("es-password")},
				}),
				beat: func() *v1beta1.Beat {
					return &beatFixture
				},
				version: "8.2.3",
			},
			want:    beatSidecarFixture(fmt.Sprintf(beatYml, "abcd1234", "es-metrics-monitoring-url")),
			wantErr: false,
		},
		{
			name: "Beat with stack monitoring enabled and remote elasticsearchRef",
			args: args{
				client: k8s.NewFakeClient(&beatFixture, &externalESSecret),
				beat: func() *v1beta1.Beat {
					beat := beatFixture.DeepCopy()
					beat.Spec.ElasticsearchRef = commonv1.ObjectSelector{SecretName: "external-es"}
					beat.Spec.Monitoring = commonv1.Monitoring{
						Metrics: commonv1.MetricsMonitoring{
							ElasticsearchRefs: []commonv1.ObjectSelector{{SecretName: "external-es"}},
						},
						Logs: commonv1.LogsMonitoring{
							ElasticsearchRefs: []commonv1.ObjectSelector{{SecretName: "external-es"}},
						},
					}
					assocConf := &commonv1.AssociationConf{
						AuthSecretName: "external-es",
						AuthSecretKey:  "password",
						URL:            esAPIFixture.URL,
					}
					for i := range beat.GetAssociations() {
						beat.GetAssociations()[i].SetAssociationConf(assocConf)
					}
					return beat
				},
				version: "8.2.3",
			},
			want:    beatSidecarFixture(fmt.Sprintf(beatYml, "QGq3wcU7Sd6bC31wh37eKQ", esAPIFixture.URL)),
			wantErr: false,
		},
		{
			name: "beat with invalid http.port configuration data returns error",
			args: args{
				client: k8s.NewFakeClient(),
				beat: func() *v1beta1.Beat {
					beat := beatFixture.DeepCopy()
					beat.Spec.Config.Data = map[string]interface{}{"http.port": "invalid"}
					return beat
				},
				version: "8.2.3",
			},
			want:    stackmon.BeatSidecar{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MetricBeat(context.Background(), tt.args.client, tt.args.beat(), tt.args.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("MetricBeat() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !cmp.Equal(got, tt.want, cmpopts.IgnoreFields(stackmon.BeatSidecar{}, "ConfigHash")) {
				t.Errorf("MetricBeat() = diff: %s", cmp.Diff(got, tt.want, cmpopts.IgnoreFields(stackmon.BeatSidecar{}, "ConfigHash")))
			}
		})
	}
}

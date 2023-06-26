// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package telemetry

import (
	"context"
	"testing"
	"time"

	"github.com/pmezard/go-difflib/difflib"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v2/pkg/about"
	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/agent/v1alpha1"
	apmv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/apm/v1"
	esav1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/autoscaling/v1alpha1"
	beatv1beta1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	entv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/enterprisesearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	mapsv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/maps/v1alpha1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/kibana"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

var (
	testOperatorInfo = about.OperatorInfo{
		OperatorUUID:            "15039433-f873-41bd-b6e7-10ee3665cafa",
		CustomOperatorNamespace: true,
		Distribution:            "v1.16.13-gke.1",
		DistributionChannel:     "test-channel",
		BuildInfo: about.BuildInfo{
			Version:  "1.1.0",
			Hash:     "b5316231",
			Date:     "2019-09-20T07:00:00Z",
			Snapshot: "true",
		},
	}

	licenceConfigMap = &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "elastic-licensing",
			Namespace: "elastic-system",
		},
		Data: map[string]string{
			"eck_license_level":         "basic",
			"enterprise_resource_units": "1",
			"timestamp":                 "2020-10-07T07:49:36+02:00",
			"total_managed_memory":      "3.22GB",
		},
	}
)

func TestMarshalTelemetry(t *testing.T) {
	for _, tt := range []struct {
		name    string
		info    about.OperatorInfo
		stats   map[string]interface{}
		license map[string]string
		want    string
	}{
		{
			name:  "empty",
			info:  about.OperatorInfo{},
			stats: nil,
			want: `eck:
  build:
    date: ""
    hash: ""
    snapshot: ""
    version: ""
  custom_operator_namespace: false
  distribution: ""
  distributionChannel: ""
  license: null
  operator_uuid: ""
  stats: null
`,
		},
		{
			name: "not empty",
			info: testOperatorInfo,
			stats: map[string]interface{}{
				"apms": map[string]interface{}{
					"pod_count":      2,
					"resource_count": 1,
				},
			},
			license: map[string]string{
				"eck_license_level": "basic",
			},
			want: `eck:
  build:
    date: "2019-09-20T07:00:00Z"
    hash: b5316231
    snapshot: "true"
    version: 1.1.0
  custom_operator_namespace: true
  distribution: v1.16.13-gke.1
  distributionChannel: test-channel
  license:
    eck_license_level: basic
  operator_uuid: 15039433-f873-41bd-b6e7-10ee3665cafa
  stats:
    apms:
      pod_count: 2
      resource_count: 1
`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gotBytes, gotErr := marshalTelemetry(context.Background(), tt.info, tt.stats, tt.license)
			require.NoError(t, gotErr)
			require.Equal(t, tt.want, string(gotBytes))
		})
	}
}

func createKbAndSecret(name, namespace string, count int32) (kbv1.Kibana, corev1.Secret) {
	kb := kbv1.Kibana{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: kbv1.KibanaSpec{
			Count: count,
		},
	}
	return kb, corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kibana.SecretName(kb),
			Namespace: namespace,
		},
	}
}

func TestNewReporter(t *testing.T) {
	kb1, s1 := createKbAndSecret("kb1", "ns1", 1)
	kb2, s2 := createKbAndSecret("kb2", "ns2", 2)
	kb3, s3 := createKbAndSecret("kb3", "ns3", 3)
	kb4, s4 := createKbAndSecret("kb4", "ns2", 1)
	kb4.Labels = map[string]string{"helm.sh/chart": "eck-kibana-0.1.0"}

	client := k8s.NewFakeClient(
		&kb1,
		&kb2,
		&kb3,
		&kb4,
		&s1,
		&s2,
		&s3,
		&s4,
		&esav1alpha1.ElasticsearchAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns1",
				Name:      "autoscaled-with-crd",
			},
		},
		&esv1.Elasticsearch{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns1",
				Name:      "autoscaled-with-annotation",
				Annotations: map[string]string{
					esv1.ElasticsearchAutoscalingSpecAnnotationName: "{}",
				},
			},
			Status: esv1.ElasticsearchStatus{
				AvailableNodes: 3,
			},
		},
		&esv1.Elasticsearch{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns1",
				Name:      "non-autoscaled",
			},
			Status: esv1.ElasticsearchStatus{
				AvailableNodes: 6,
			},
		},
		&esv1.Elasticsearch{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns1",
				Name:      "monitored",
			},
			Spec: esv1.ElasticsearchSpec{
				Monitoring: commonv1.Monitoring{
					Logs:    commonv1.LogsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "monitoring"}}},
					Metrics: commonv1.MetricsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "monitoring"}}},
				},
			},
			Status: esv1.ElasticsearchStatus{
				AvailableNodes: 1,
			},
		},
		&esv1.Elasticsearch{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns1",
				Name:      "helm-managed",
				Labels:    map[string]string{"helm.sh/chart": "eck-elasticsearch-0.1.0"},
			},
		},
		&apmv1.ApmServer{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns1",
			},
			Status: apmv1.ApmServerStatus{
				DeploymentStatus: commonv1.DeploymentStatus{
					AvailableNodes: 2,
				},
			},
		},
		&entv1.EnterpriseSearch{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns1",
			},
			Status: entv1.EnterpriseSearchStatus{
				DeploymentStatus: commonv1.DeploymentStatus{
					AvailableNodes: 3,
				},
			},
		},
		&logstashv1alpha1.Logstash{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns1",
			},
			Spec: logstashv1alpha1.LogstashSpec{
				Count: 3,
				Monitoring: commonv1.Monitoring{
					Logs:    commonv1.LogsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "monitoring"}}},
					Metrics: commonv1.MetricsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "monitoring"}}},
				},
				Pipelines: []commonv1.Config{
					{Data: map[string]interface{}{"pipeline.id": "main"}},
				},
				Services: []logstashv1alpha1.LogstashService{
					{
						Name: "test1",
						Service: commonv1.ServiceTemplate{
							Spec: corev1.ServiceSpec{
								Ports: []corev1.ServicePort{
									{Port: 9200},
								},
							},
						},
					},
					{
						Name: "test2",
						Service: commonv1.ServiceTemplate{
							Spec: corev1.ServiceSpec{
								Ports: []corev1.ServicePort{
									{Port: 9201},
								},
							},
						},
					},
				},
			},
			Status: logstashv1alpha1.LogstashStatus{
				AvailableNodes: 3,
			},
		},
		&logstashv1alpha1.Logstash{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns2",
			},
			Spec: logstashv1alpha1.LogstashSpec{
				Count: 1,
				Services: []logstashv1alpha1.LogstashService{
					{
						Name: "test1",
						Service: commonv1.ServiceTemplate{
							Spec: corev1.ServiceSpec{
								Ports: []corev1.ServicePort{
									{Port: 9200},
								},
							},
						},
					},
				},
			},
			Status: logstashv1alpha1.LogstashStatus{
				AvailableNodes: 1,
			},
		},
		&beatv1beta1.Beat{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "beat1",
				Namespace: "ns1",
			},
			Spec: beatv1beta1.BeatSpec{
				Type: "filebeat",
			},
			Status: beatv1beta1.BeatStatus{
				AvailableNodes: 7,
			},
		},
		&beatv1beta1.Beat{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "beat2",
				Namespace: "ns2",
			},
			Spec: beatv1beta1.BeatSpec{
				Type: "metricbeat",
			},
			Status: beatv1beta1.BeatStatus{
				AvailableNodes: 1,
			},
		},
		&beatv1beta1.Beat{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "beat3",
				Namespace: "ns3",
			},
			Spec: beatv1beta1.BeatSpec{
				Type: "metricbeat",
			},
			Status: beatv1beta1.BeatStatus{
				AvailableNodes: 7,
			},
		},
		&agentv1alpha1.Agent{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "agent1",
				Namespace: "ns2",
			},
			Status: agentv1alpha1.AgentStatus{
				AvailableNodes: 10,
			},
		},
		&agentv1alpha1.Agent{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "agent2",
				Namespace: "ns2",
			},
			Spec: agentv1alpha1.AgentSpec{
				ElasticsearchRefs: []agentv1alpha1.Output{{}, {}}, // two outputs
			},
			Status: agentv1alpha1.AgentStatus{
				AvailableNodes: 6,
			},
		},
		&agentv1alpha1.Agent{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "agent3",
				Namespace: "ns2",
			},
			Spec: agentv1alpha1.AgentSpec{
				FleetServerEnabled: true,
				Mode:               agentv1alpha1.AgentFleetMode,
			},
			Status: agentv1alpha1.AgentStatus{
				AvailableNodes: 3,
			},
		},
		&agentv1alpha1.Agent{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "agent4",
				Namespace: "ns2",
			},
			Spec: agentv1alpha1.AgentSpec{
				Mode: agentv1alpha1.AgentFleetMode,
			},
			Status: agentv1alpha1.AgentStatus{
				AvailableNodes: 5,
			},
		},
		&mapsv1alpha1.ElasticMapsServer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "maps1",
				Namespace: "ns1",
			},
			Status: mapsv1alpha1.MapsStatus{
				DeploymentStatus: commonv1.DeploymentStatus{
					AvailableNodes: 1,
				},
			},
		},
		licenceConfigMap,
		&policyv1alpha1.StackConfigPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "scp1",
				Namespace: "ns1",
			},
			Spec: policyv1alpha1.StackConfigPolicySpec{
				Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
					ClusterSettings: &commonv1.Config{Data: map[string]interface{}{
						"indices.recovery.max_bytes_per_sec": "100mb",
					}},
					SnapshotRepositories: &commonv1.Config{Data: map[string]interface{}{
						"repo1": "settings...",
					}},
					SnapshotLifecyclePolicies: &commonv1.Config{Data: map[string]interface{}{
						"slm1": "settings...",
						"slm2": "settings...",
					}},
				},
			},
			Status: policyv1alpha1.StackConfigPolicyStatus{
				Resources: 10,
			},
		},
		&policyv1alpha1.StackConfigPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "scp2",
				Namespace: "ns2",
			},
			Spec: policyv1alpha1.StackConfigPolicySpec{
				Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
					SnapshotRepositories: &commonv1.Config{Data: map[string]interface{}{
						"repo1": "settings...",
					}},
					SnapshotLifecyclePolicies: &commonv1.Config{Data: map[string]interface{}{
						"slm1": "settings...",
					}},
				},
			},
			Status: policyv1alpha1.StackConfigPolicyStatus{
				Resources: 5,
			},
		},
	)

	// We only want the reporter to handle the managed namespaces, in this test only ns1 and ns2 are managed.
	r := NewReporter(testOperatorInfo, client, "elastic-system", []string{kb1.Namespace, kb2.Namespace}, 1*time.Hour, nil)
	r.report(context.Background())

	wantData := map[string][]byte{
		"telemetry.yml": []byte(`eck:
  build:
    date: "2019-09-20T07:00:00Z"
    hash: b5316231
    snapshot: "true"
    version: 1.1.0
  custom_operator_namespace: true
  distribution: v1.16.13-gke.1
  distributionChannel: test-channel
  license:
    eck_license_level: basic
    enterprise_resource_units: "1"
    total_managed_memory: 3.22GB
  operator_uuid: 15039433-f873-41bd-b6e7-10ee3665cafa
  stats:
    agents:
      fleet_mode: 2
      fleet_server: 1
      multiple_refs: 1
      pod_count: 24
      resource_count: 4
    apms:
      pod_count: 2
      resource_count: 1
    beats:
      auditbeat_count: 0
      filebeat_count: 1
      heartbeat_count: 0
      journalbeat_count: 0
      metricbeat_count: 1
      packetbeat_count: 0
      pod_count: 8
      resource_count: 2
    elasticsearches:
      autoscaled_resource_count: 2
      helm_resource_count: 1
      pod_count: 10
      resource_count: 4
      stack_monitoring_logs_count: 1
      stack_monitoring_metrics_count: 1
    enterprisesearches:
      pod_count: 3
      resource_count: 1
    kibanas:
      helm_resource_count: 1
      pod_count: 0
      resource_count: 3
    logstashes:
      pipeline_count: 1
      pipeline_ref_count: 0
      pod_count: 4
      resource_count: 2
      service_count: 3
      stack_monitoring_logs_count: 1
      stack_monitoring_metrics_count: 1
    maps:
      pod_count: 1
      resource_count: 1
    stackconfigpolicies:
      configured_resources_count: 15
      resource_count: 2
      settings:
        cluster_settings_count: 1
        component_templates_count: 0
        composable_index_templates_count: 0
        index_lifecycle_policies_count: 0
        ingest_pipelines_count: 0
        role_mappings_count: 0
        snapshot_lifecycle_policies_count: 3
        snapshot_repositories_count: 2
`),
	}

	require.NoError(t, client.Get(context.Background(), k8s.ExtractNamespacedName(&s1), &s1))
	assertSameSecretContent(t, wantData, s1.Data)

	// We expect Kibana instances in s1 and s2 to have the same content.
	require.NoError(t, client.Get(context.Background(), k8s.ExtractNamespacedName(&s2), &s2))
	assertSameSecretContent(t, wantData, s2.Data)

	// No data expected in s3 since it is not a managed namespace.
	require.NoError(t, client.Get(context.Background(), k8s.ExtractNamespacedName(&s3), &s3))
	require.Nil(t, s3.Data)
}

// assertSameSecretContent compares 2 data secrets and print a human friendly diff if not equal.
func assertSameSecretContent(t *testing.T, expectedData, actualData map[string][]byte) {
	t.Helper()
	require.Equal(t, len(expectedData), len(actualData))
	for k, expected := range expectedData {
		actual, exists := actualData[k]
		require.True(t, exists)
		diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
			A:        difflib.SplitLines(string(expected)),
			B:        difflib.SplitLines(string(actual)),
			FromFile: "expected",
			ToFile:   "actual",
			Context:  3,
		})
		require.NoError(t, err)
		require.Equal(t, string(expected), string(actual), "unexpected content for %s, diff:\n%s", k, diff)
	}
}

// TestReporter_report allows testing different combinations of resources.
func TestReporter_report(t *testing.T) {
	testNS := "ns1"
	type fields struct {
		objects []client.Object
	}
	tests := []struct {
		name     string
		fields   fields
		wantData TemplateData
	}{
		{
			name: "With metrics monitoring only",
			fields: fields{
				objects: []client.Object{
					&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: testNS,
							Name:      "non-autoscaled",
						},
						Status: esv1.ElasticsearchStatus{
							AvailableNodes: 6,
						},
					},
					&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: testNS,
							Name:      "monitored",
						},
						Spec: esv1.ElasticsearchSpec{
							Monitoring: commonv1.Monitoring{
								Metrics: commonv1.MetricsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "monitoring"}}},
							},
						},
						Status: esv1.ElasticsearchStatus{
							AvailableNodes: 1,
						},
					},
					&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: testNS,
							Name:      "monitored2",
						},
						Spec: esv1.ElasticsearchSpec{
							Monitoring: commonv1.Monitoring{
								Metrics: commonv1.MetricsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "monitoring"}}},
							},
						},
						Status: esv1.ElasticsearchStatus{
							AvailableNodes: 2,
						},
					},
				},
			},
			wantData: TemplateData{
				ElasticsearchTemplateData{
					PodCount:                    9,
					ResourceCount:               3,
					StackMonitoringMetricsCount: 2,
				},
			},
		},
		{
			name: "With log monitoring only",
			fields: fields{
				objects: []client.Object{
					&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: testNS,
							Name:      "non-autoscaled",
						},
						Status: esv1.ElasticsearchStatus{
							AvailableNodes: 2,
						},
					},
					&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: testNS,
							Name:      "monitored",
						},
						Spec: esv1.ElasticsearchSpec{
							Monitoring: commonv1.Monitoring{
								Logs: commonv1.LogsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "monitoring"}}},
							},
						},
						Status: esv1.ElasticsearchStatus{
							AvailableNodes: 8,
						},
					},
				},
			},
			wantData: TemplateData{
				ElasticsearchTemplateData{
					PodCount:                 10,
					ResourceCount:            2,
					StackMonitoringLogsCount: 1,
				},
			},
		}, {
			name: "With downward API and one label",
			fields: fields{
				objects: []client.Object{
					&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: testNS,
							Name:      "node-labels1",
							Annotations: map[string]string{
								esv1.DownwardNodeLabelsAnnotation: "ns/label1",
							},
						},
						Status: esv1.ElasticsearchStatus{
							AvailableNodes: 2,
						},
					},
					&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: testNS,
							Name:      "simple",
						},
						Status: esv1.ElasticsearchStatus{
							AvailableNodes: 2,
						},
					},
				},
			},
			wantData: TemplateData{
				ElasticsearchTemplateData{
					PodCount:      4,
					ResourceCount: 2,
					NodeLabelsTemplateData: &NodeLabelsTemplateData{
						ResourceWithNodeLabelsCount: 1,
						DistinctNodeLabelsCount:     1,
					},
				},
			},
		}, {
			name: "With downward API and several labels",
			fields: fields{
				objects: []client.Object{
					&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: testNS,
							Name:      "node-labels1",
							Annotations: map[string]string{
								esv1.DownwardNodeLabelsAnnotation: "ns/label1",
							},
						},
						Status: esv1.ElasticsearchStatus{
							AvailableNodes: 2,
						},
					},
					&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: testNS,
							Name:      "node-labels2",
							Annotations: map[string]string{
								esv1.DownwardNodeLabelsAnnotation: "ns/label2,ns/label1,ns/label3",
							},
						},
						Status: esv1.ElasticsearchStatus{
							AvailableNodes: 2,
						},
					},
				},
			},
			wantData: TemplateData{
				ElasticsearchTemplateData{
					PodCount:      4,
					ResourceCount: 2,
					NodeLabelsTemplateData: &NodeLabelsTemplateData{
						ResourceWithNodeLabelsCount: 2,
						DistinctNodeLabelsCount:     3, // ns/label1,ns/label2 and ns/label3
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kb1, s1 := createKbAndSecret("kb1", testNS, 1)
			client := k8s.NewFakeClient(append(tt.fields.objects, &kb1, &s1, licenceConfigMap)...)
			r := &Reporter{
				operatorInfo:      testOperatorInfo,
				client:            client,
				operatorNamespace: "elastic-system",
				managedNamespaces: []string{testNS},
				telemetryInterval: 1 * time.Hour,
			}
			r.report(context.Background())
			require.NoError(t, client.Get(context.Background(), k8s.ExtractNamespacedName(&s1), &s1))
			wantData := map[string][]byte{"telemetry.yml": renderExpectedTemplate(t, tt.wantData)}
			assertSameSecretContent(t, wantData, s1.Data)
		})
	}
}

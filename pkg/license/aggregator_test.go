// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package license

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/agent/v1alpha1"
	apmv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	entv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/enterprisesearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	lsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/agent"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana"
	kblabel "github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func TestMemFromJavaOpts(t *testing.T) {
	tests := []struct {
		name     string
		actual   string
		expected resource.Quantity
		isErr    bool
	}{
		{
			name:     "in k",
			actual:   "-Xms1k -Xmx8388608k",
			expected: resource.MustParse("16777216Ki"),
		},
		{
			name:     "in K",
			actual:   "-Xmx1024K",
			expected: resource.MustParse("2048Ki"),
		},
		{
			name:     "in m",
			actual:   "-Xmx512m -Xms256m",
			expected: resource.MustParse("1024Mi"),
		},
		{
			name:     "in M",
			actual:   "-Xmx256M",
			expected: resource.MustParse("512Mi"),
		},
		{
			name:     "in g",
			actual:   "-Xmx64g",
			expected: resource.MustParse("128Gi"),
		},
		{
			name:     "in G",
			actual:   "-Xmx64G",
			expected: resource.MustParse("128Gi"),
		},
		{
			name:     "without unit",
			actual:   "-Xmx1048576",
			expected: resource.MustParse("2Mi"),
		},
		{
			name:     "with trailing spaces at the end",
			actual:   "-Xms1k -Xmx8388608k   ",
			expected: resource.MustParse("16777216Ki"),
		},
		{
			name:     "with trailing space at the beginning",
			actual:   "  -Xms1k -Xmx8388608k",
			expected: resource.MustParse("16777216Ki"),
		},
		{
			name:     "no memory setting detected",
			actual:   "-Dlog4j2.formatMsgNoLookups=true",
			expected: resource.MustParse("0"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := memFromJavaOpts(tt.actual)
			if tt.isErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if !got.Equal(tt.expected) {
					t.Errorf("memFromJavaOpts(%s) = %v, want %s", tt.actual, got.String(), tt.expected.String())
				}
			}
		})
	}
}

func TestMemFromNodeOpts(t *testing.T) {
	tests := []struct {
		name     string
		actual   string
		expected string
		isErr    bool
	}{
		{
			name:     "with max-old-space-size option",
			actual:   "--max-old-space-size=2048",
			expected: "2048M",
		},
		{
			name:   "empty options",
			actual: "",
			isErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := memFromNodeOptions(tt.actual)
			if tt.isErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				got := resource.MustParse(tt.expected)
				if !got.Equal(q) {
					t.Errorf("memFromNodeOptions(%s) = %v, want %s", tt.actual, got, tt.expected)
				}
			}
		})
	}
}

func TestAggregator(t *testing.T) {
	tests := []struct {
		name      string
		testdata  string          // optional YAML file with Elastic CRs
		objects   []client.Object // objects added to the fake client alongside the testdata, e.g. Kibana deployments
		wantUsage map[string]float64
		wantTotal float64
	}{
		{
			name:     "stack",
			testdata: "testdata/stack.yaml",
			wantUsage: map[string]float64{
				elasticsearchKey: 294.0,
				kibanaKey:        5.9073486328125, // 4Gi + 2048M (node options)
				apmKey:           2.0,
				entSearchKey:     24.0,
				logstashKey:      4.0,
			},
			wantTotal: 329.9073486328125,
		},
		{
			name:     "resources shorthand",
			testdata: "testdata/resources_shorthand.yaml",
			wantUsage: map[string]float64{
				elasticsearchKey: 20.0, // 2x4Gi (shorthand) + 4Gi (pod template) + 2Gi (shorthand over pod template) + 6Gi (shorthand over heap)
				kibanaKey:        6.0,  // 2x3Gi (shorthand)
				apmKey:           1.0,
				entSearchKey:     5.0,
				logstashKey:      3.0,
			},
			wantTotal: 35.0,
		},
		{
			name: "kibana replicas from the deployment",
			objects: []client.Object{
				// the spec count is 2 but the deployment is scaled to 5
				&kbv1.Kibana{Name: "test-kibana", Namespace: "default", Spec: kbv1.KibanaSpec{
					Count:     2,
					Resources: commonv1.Resources{Limits: commonv1.ResourceAllocations{Memory: new(resource.MustParse("1Gi"))}},
				}},
				kibanaDeployment("test-kibana", kblabel.SinglePoolRole, 5),
			},
			wantUsage: map[string]float64{kibanaKey: 5.0}, // 5x1Gi
			wantTotal: 5.0,
		},
		{
			name: "kibana background tasks pool",
			objects: []client.Object{
				// a Kibana with background task isolation enabled renders one Deployment per pool, each with its own
				// pod template, resources and replicas, the aggregator must sum both
				&kbv1.Kibana{Name: "test-kibana", Namespace: "default", Spec: kbv1.KibanaSpec{
					Count: 3,
					PodTemplate: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{
						Name: kbv1.KibanaContainerName,
						Env:  []corev1.EnvVar{{Name: kibana.EnvNodeOptions, Value: "--max-old-space-size=1024"}},
					}}}},
					BackgroundTasks: &kbv1.KibanaBackgroundTasks{
						// no count: the replicas are managed by an HPA and read from the deployment
						Resources: commonv1.Resources{Limits: commonv1.ResourceAllocations{Memory: new(resource.MustParse("4Gi"))}},
					},
				}},
				kibanaDeployment("test-kibana", kblabel.UIRole, 3),
				kibanaDeployment("test-kibana", kblabel.BackgroundTasksRole, 2),
			},
			wantUsage: map[string]float64{
				kibanaKey: 3*1.024e9/(1<<30) + 8.0, // 3x1024M (ui, node options) + 2x4Gi (background tasks, shorthand over node options)
			},
			wantTotal: 3*1.024e9/(1<<30) + 8.0,
		},
		{
			name: "kibana pools without deployments fall back to the spec counts",
			objects: []client.Object{
				&kbv1.Kibana{Name: "test-kibana", Namespace: "default", Spec: kbv1.KibanaSpec{
					Count:           3,
					BackgroundTasks: &kbv1.KibanaBackgroundTasks{Count: ptr.To[int32](2)},
				}},
				&kbv1.Kibana{Name: "test-kibana-hpa", Namespace: "default", Spec: kbv1.KibanaSpec{
					Count:           1,
					BackgroundTasks: &kbv1.KibanaBackgroundTasks{}, // no count, the deployment would get the Kubernetes default of 1
				}},
			},
			wantUsage: map[string]float64{
				kibanaKey: 14.0, // (3 + 2 + 1 + 1) x 2Gi (default)
			},
			wantTotal: 14.0,
		},
		{
			name: "fleet server agents",
			objects: []client.Object{
				// a regular Elastic Agent is not billable
				&agentv1alpha1.Agent{Name: "test-agent", Namespace: "default", Spec: agentv1alpha1.AgentSpec{
					DaemonSet: &agentv1alpha1.DaemonSetSpec{},
				}},
				// a Fleet Server Deployment counts its spec replicas with the fleet default memory limit
				&agentv1alpha1.Agent{Name: "fs-deployment", Namespace: "default", Spec: agentv1alpha1.AgentSpec{
					FleetServerEnabled: true,
					Deployment:         &agentv1alpha1.DeploymentSpec{Replicas: ptr.To[int32](2)},
				}},
				// a Fleet Server StatefulSet without replicas defaults to 1, with the resources shorthand
				&agentv1alpha1.Agent{Name: "fs-statefulset", Namespace: "default", Spec: agentv1alpha1.AgentSpec{
					FleetServerEnabled: true,
					StatefulSet:        &agentv1alpha1.StatefulSetSpec{},
					Resources:          commonv1.Resources{Limits: commonv1.ResourceAllocations{Memory: new(resource.MustParse("2Gi"))}},
				}},
				// a Fleet Server DaemonSet counts 0 until the DaemonSet exists, the number of nodes is unknown
				&agentv1alpha1.Agent{Name: "fs-daemonset", Namespace: "default", Spec: agentv1alpha1.AgentSpec{
					FleetServerEnabled: true,
					DaemonSet:          &agentv1alpha1.DaemonSetSpec{},
				}},
			},
			wantUsage: map[string]float64{fleetServerKey: 4.0}, // 2x1Gi (deployment) + 1x2Gi (statefulset)
			wantTotal: 4.0,
		},
		{
			name: "fleet server replicas from the live workload",
			objects: []client.Object{
				// the spec declares 1 replica but the deployment is scaled to 3
				&agentv1alpha1.Agent{Name: "fs-deployment", Namespace: "default", Spec: agentv1alpha1.AgentSpec{
					FleetServerEnabled: true,
					Deployment:         &agentv1alpha1.DeploymentSpec{Replicas: ptr.To[int32](1)},
				}},
				&appsv1.Deployment{
					Name: agent.Name("fs-deployment"), Namespace: "default",
					Spec: appsv1.DeploymentSpec{Replicas: ptr.To[int32](3)},
				},
				// a DaemonSet schedules one pod per node, read from the DaemonSet status
				&agentv1alpha1.Agent{Name: "fs-daemonset", Namespace: "default", Spec: agentv1alpha1.AgentSpec{
					FleetServerEnabled: true,
					DaemonSet: &agentv1alpha1.DaemonSetSpec{PodTemplate: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name:      agentv1alpha1.AgentContainerName,
							Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("512Mi")}},
						}},
					}}},
				}},
				&appsv1.DaemonSet{
					Name: agent.Name("fs-daemonset"), Namespace: "default",
					Status: appsv1.DaemonSetStatus{DesiredNumberScheduled: 4},
				},
			},
			wantUsage: map[string]float64{fleetServerKey: 5.0}, // 3x1Gi (deployment) + 4x512Mi (daemonset)
			wantTotal: 5.0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := tt.objects
			if tt.testdata != "" {
				objects = append(readObjects(t, tt.testdata), objects...)
			}
			aggregator := aggregator{client: k8s.NewFakeClient(objects...)}

			val, err := aggregator.aggregateMemory(context.Background())
			require.NoError(t, err)
			for k, v := range tt.wantUsage {
				require.Equal(t, v, val.appUsage[k].inGiB(), k)
			}
			require.Equal(t, tt.wantTotal, val.totalMemory.inGiB(), "total")
		})
	}
}

func TestContainerMemLimits(t *testing.T) {
	quantity := func(s string) *resource.Quantity {
		q := resource.MustParse(s)
		return &q
	}
	podTemplate := func(limit string, request string, javaOpts string) corev1.PodTemplateSpec {
		c := corev1.Container{Name: "main"}
		if limit != "" {
			c.Resources.Limits = corev1.ResourceList{corev1.ResourceMemory: resource.MustParse(limit)}
		}
		if request != "" {
			c.Resources.Requests = corev1.ResourceList{corev1.ResourceMemory: resource.MustParse(request)}
		}
		if javaOpts != "" {
			c.Env = []corev1.EnvVar{{Name: "JAVA_OPTS", Value: javaOpts}}
		}
		return corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{c}}}
	}
	defaultLimit := resource.MustParse("2Gi")
	tests := []struct {
		name        string
		podTemplate corev1.PodTemplateSpec
		overrides   commonv1.Resources
		want        resource.Quantity
	}{
		{
			name:        "default when nothing is set",
			podTemplate: podTemplate("", "", ""),
			want:        resource.MustParse("2Gi"),
		},
		{
			name:        "default when there is no main container in the pod template",
			podTemplate: corev1.PodTemplateSpec{},
			want:        resource.MustParse("2Gi"),
		},
		{
			name:        "pod template limit",
			podTemplate: podTemplate("4Gi", "", ""),
			want:        resource.MustParse("4Gi"),
		},
		{
			name:        "heap size when no limit is set",
			podTemplate: podTemplate("", "", "-Xmx3g"),
			want:        resource.MustParse("6Gi"),
		},
		{
			name:        "heap size when only a request is set",
			podTemplate: podTemplate("", "1Gi", "-Xmx3g"),
			want:        resource.MustParse("6Gi"),
		},
		{
			name:        "shorthand when the pod template sets nothing",
			podTemplate: podTemplate("", "", ""),
			overrides:   commonv1.Resources{Limits: commonv1.ResourceAllocations{Memory: quantity("4Gi")}},
			want:        resource.MustParse("4Gi"),
		},
		{
			name:        "shorthand overrides the pod template limit",
			podTemplate: podTemplate("8Gi", "", ""),
			overrides:   commonv1.Resources{Limits: commonv1.ResourceAllocations{Memory: quantity("2Gi")}},
			want:        resource.MustParse("2Gi"),
		},
		{
			name:        "shorthand overrides the heap size",
			podTemplate: podTemplate("", "", "-Xmx3g"),
			overrides:   commonv1.Resources{Limits: commonv1.ResourceAllocations{Memory: quantity("1Gi")}},
			want:        resource.MustParse("1Gi"),
		},
		{
			name:        "shorthand request only falls back to the heap size",
			podTemplate: podTemplate("", "", "-Xmx3g"),
			overrides:   commonv1.Resources{Requests: commonv1.ResourceAllocations{Memory: quantity("1Gi")}},
			want:        resource.MustParse("6Gi"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := containerMemLimits(
				containersWithResourceOverrides("main", tt.podTemplate, tt.overrides),
				"main", "JAVA_OPTS", memFromJavaOpts, defaultLimit,
			)
			require.NoError(t, err)
			assert.Equal(t, 0, tt.want.Cmp(got), "want %s, got %s", tt.want.String(), got.String())
		})
	}
}

// kibanaDeployment builds the Deployment of the given Kibana pool, named as the Kibana controller names it,
// carrying only the replicas the license aggregator reads from it.
func kibanaDeployment(kbName string, role kblabel.Role, replicas int32) *appsv1.Deployment {
	name := kbv1.Deployment(kbName)
	if role.IsBackgroundTasks() {
		name = kbv1.BackgroundTasksDeployment(kbName)
	}
	return &appsv1.Deployment{
		Name: name, Namespace: "default",
		Spec: appsv1.DeploymentSpec{Replicas: new(replicas)},
	}
}

func readObjects(t *testing.T, filePath string) []client.Object {
	t.Helper()

	scheme := runtime.NewScheme()
	scheme.AddKnownTypes(esv1.GroupVersion, &esv1.Elasticsearch{}, &esv1.ElasticsearchList{})
	scheme.AddKnownTypes(kbv1.GroupVersion, &kbv1.Kibana{}, &kbv1.KibanaList{})
	scheme.AddKnownTypes(apmv1.GroupVersion, &apmv1.ApmServer{}, &apmv1.ApmServerList{})
	scheme.AddKnownTypes(entv1.GroupVersion, &entv1.EnterpriseSearch{}, &entv1.EnterpriseSearchList{})
	scheme.AddKnownTypes(lsv1alpha1.GroupVersion, &lsv1alpha1.Logstash{}, &lsv1alpha1.LogstashList{})
	decoder := serializer.NewCodecFactory(scheme).UniversalDeserializer()

	f, err := os.Open(filePath)
	require.NoError(t, err)

	defer f.Close()

	yamlReader := yaml.NewYAMLReader(bufio.NewReader(f))

	var objects []client.Object

	for {
		yamlBytes, err := yamlReader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			require.NoError(t, err)
		}
		runtimeObj, _, err := decoder.Decode(yamlBytes, nil, nil)
		require.NoError(t, err)

		obj, ok := runtimeObj.(client.Object)
		require.True(t, ok)

		objects = append(objects, obj)
	}

	return objects
}

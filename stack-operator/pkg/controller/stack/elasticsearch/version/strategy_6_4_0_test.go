package version

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/stack-operator/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/common/version"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/elasticsearch"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/elasticsearch/client"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

var testProbeUser = client.User{Name: "username", Password: "supersecure"}
var testObjectMeta = v1.ObjectMeta{
	Name:      "my-stack",
	Namespace: "default",
}

var testStrategy_6_4_0 = newStrategy_6_4_0(version.Version{Major: 6, Minor: 4, Patch: 2})

func TestNewEnvironmentVars(t *testing.T) {
	type args struct {
		p                      elasticsearch.NewPodSpecParams
		extraFilesSecretVolume elasticsearch.SecretVolume
	}

	tests := []struct {
		name          string
		args          args
		wantEnvSubset []corev1.EnvVar
	}{
		{name: "2 nodes",
			args: args{
				p: elasticsearch.NewPodSpecParams{
					ClusterName:                    "cluster",
					CustomImageName:                "myImage",
					DiscoveryServiceName:           "discovery-service",
					DiscoveryZenMinimumMasterNodes: 3,
					NodeTypes: deploymentsv1alpha1.NodeTypesSpec{
						Master: true,
						Data:   true,
						Ingest: false,
						ML:     true,
					},
					SetVMMaxMapCount: true,
					Version:          "1.2.3",
					ProbeUser:        testProbeUser,
				},
				extraFilesSecretVolume: elasticsearch.SecretVolume{},
			},
			wantEnvSubset: []corev1.EnvVar{
				{Name: "discovery.zen.ping.unicast.hosts", Value: "discovery-service"},
				{Name: "cluster.name", Value: "cluster"},
				{Name: "discovery.zen.minimum_master_nodes", Value: "3"},
				{Name: "network.host", Value: "0.0.0.0"},
				{Name: "path.data", Value: "/usr/share/elasticsearch/data"},
				{Name: "path.logs", Value: "/usr/share/elasticsearch/logs"},
				{Name: "ES_JAVA_OPTS", Value: "-Xms512M -Xmx512M"},
				{Name: "node.master", Value: "true"},
				{Name: "node.data", Value: "true"},
				{Name: "node.ingest", Value: "false"},
				{Name: "node.ml", Value: "true"},
				{Name: "xpack.security.enabled", Value: "true"},
				{Name: "xpack.license.self_generated.type", Value: "trial"},
				{Name: "xpack.security.authc.reserved_realm.enabled", Value: "false"},
				{Name: "PROBE_USERNAME", Value: "username"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := testStrategy_6_4_0.newEnvironmentVars(
				tt.args.p, tt.args.extraFilesSecretVolume,
			)
			for _, v := range tt.wantEnvSubset {
				assert.Contains(t, got, v)
			}
		})
	}
}

func TestCreateExpectedPodSpecsReturnsCorrectNodeCount(t *testing.T) {
	tests := []struct {
		name             string
		stack            deploymentsv1alpha1.Stack
		expectedPodCount int
	}{
		{
			name: "2 nodes stack",
			stack: deploymentsv1alpha1.Stack{
				ObjectMeta: testObjectMeta,
				Spec: deploymentsv1alpha1.StackSpec{
					Elasticsearch: deploymentsv1alpha1.ElasticsearchSpec{
						Topologies: []deploymentsv1alpha1.ElasticsearchTopologySpec{
							{
								NodeCount: 2,
							},
						},
					},
				},
			},
			expectedPodCount: 2,
		},
		{
			name: "1 master 2 data",
			stack: deploymentsv1alpha1.Stack{
				ObjectMeta: testObjectMeta,
				Spec: deploymentsv1alpha1.StackSpec{
					Elasticsearch: deploymentsv1alpha1.ElasticsearchSpec{
						Topologies: []deploymentsv1alpha1.ElasticsearchTopologySpec{
							{
								NodeCount: 1,
								NodeTypes: deploymentsv1alpha1.NodeTypesSpec{Master: true},
							},
							{
								NodeCount: 2,
								NodeTypes: deploymentsv1alpha1.NodeTypesSpec{Data: true},
							},
						},
					},
				},
			},
			expectedPodCount: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			podSpecs, err := testStrategy_6_4_0.NewExpectedPodSpecs(
				tt.stack,
				elasticsearch.NewPodSpecParams{ProbeUser: testProbeUser},
			)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedPodCount, len(podSpecs))
		})
	}
}

func TestCreateExpectedPodSpecsReturnsCorrectPodSpec(t *testing.T) {
	stack := deploymentsv1alpha1.Stack{
		ObjectMeta: testObjectMeta,
		Spec: deploymentsv1alpha1.StackSpec{
			Version: "1.2.3",
			Elasticsearch: deploymentsv1alpha1.ElasticsearchSpec{
				Image:            "custom-image",
				SetVMMaxMapCount: true,
				Topologies: []deploymentsv1alpha1.ElasticsearchTopologySpec{
					{
						NodeCount: 1,
						NodeTypes: deploymentsv1alpha1.NodeTypesSpec{Master: true},
					},
				},
			},
		},
	}
	podSpec, err := testStrategy_6_4_0.NewExpectedPodSpecs(
		stack,
		elasticsearch.NewPodSpecParams{ProbeUser: testProbeUser},
	)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(podSpec))

	esPodSpec := podSpec[0].PodSpec
	assert.Equal(t, 1, len(esPodSpec.Containers))
	assert.Equal(t, 2, len(esPodSpec.InitContainers))
	assert.Equal(t, 8, len(esPodSpec.Volumes))

	esContainer := esPodSpec.Containers[0]
	assert.NotEqual(t, 0, esContainer.Env)
	// esContainer.Env actual values are tested in environment_test.go
	assert.Equal(t, "custom-image", esContainer.Image)
	assert.NotNil(t, esContainer.ReadinessProbe)
	assert.ElementsMatch(t, elasticsearch.DefaultContainerPorts, esContainer.Ports)
	assert.Equal(t, 8, len(esContainer.VolumeMounts))
	assert.NotEmpty(t, esContainer.ReadinessProbe.Handler.Exec.Command)
}

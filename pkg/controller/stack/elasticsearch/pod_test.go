package elasticsearch

import (
	"testing"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/client"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var probeUser = client.User{Name: "username", Password: "supersecure"}
var objectMeta = metav1.ObjectMeta{
	Name:      "my-stack",
	Namespace: "default",
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
				ObjectMeta: objectMeta,
				Spec: deploymentsv1alpha1.StackSpec{
					Elasticsearch: deploymentsv1alpha1.ElasticsearchSpec{
						Topologies: []deploymentsv1alpha1.ElasticsearchTopologySpec{
							deploymentsv1alpha1.ElasticsearchTopologySpec{
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
				ObjectMeta: objectMeta,
				Spec: deploymentsv1alpha1.StackSpec{
					Elasticsearch: deploymentsv1alpha1.ElasticsearchSpec{
						Topologies: []deploymentsv1alpha1.ElasticsearchTopologySpec{
							deploymentsv1alpha1.ElasticsearchTopologySpec{
								NodeCount: 1,
								NodeTypes: deploymentsv1alpha1.NodeTypesSpec{Master: true},
							},
							deploymentsv1alpha1.ElasticsearchTopologySpec{
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
			podSpecs, err := CreateExpectedPodSpecs(tt.stack, probeUser, NewPodExtraParams{})
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedPodCount, len(podSpecs))
		})
	}
}

func TestCreateExpectedPodSpecsReturnsCorrectPodSpec(t *testing.T) {
	stack := deploymentsv1alpha1.Stack{
		ObjectMeta: objectMeta,
		Spec: deploymentsv1alpha1.StackSpec{
			Version: "1.2.3",
			Elasticsearch: deploymentsv1alpha1.ElasticsearchSpec{
				Image:            "custom-image",
				SetVMMaxMapCount: true,
				Topologies: []deploymentsv1alpha1.ElasticsearchTopologySpec{
					deploymentsv1alpha1.ElasticsearchTopologySpec{
						NodeCount: 1,
						NodeTypes: deploymentsv1alpha1.NodeTypesSpec{Master: true},
					},
				},
			},
		},
	}
	podSpec, err := CreateExpectedPodSpecs(stack, probeUser, NewPodExtraParams{})
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
	assert.ElementsMatch(t, defaultContainerPorts, esContainer.Ports)
	assert.Equal(t, 8, len(esContainer.VolumeMounts))
	assert.NotEmpty(t, esContainer.ReadinessProbe.Handler.Exec.Command)
}

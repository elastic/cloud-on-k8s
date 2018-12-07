package stack

import (
	"fmt"
	"testing"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/test/e2e/helpers"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // auth on gke
)

// CreationTestSteps tests the creation of the given stack.
// The stack is not deleted at the end.
func CreationTestSteps(stack v1alpha1.Stack, k *helpers.K8sHelper) []helpers.TestStep {
	stackKey := types.NamespacedName{
		Name:      stack.GetName(),
		Namespace: stack.GetNamespace(),
	}

	return []helpers.TestStep{

		{
			Name: "Creating a stack should succeed",
			Test: func(t *testing.T) {
				err := k.Client.Create(helpers.DefaultCtx, &stack)
				assert.NoError(t, err)
			},
		},

		{
			Name: "Stack should be created",
			Test: func(t *testing.T) {
				var createdStack v1alpha1.Stack
				err := k.Client.Get(helpers.DefaultCtx, stackKey, &createdStack)
				assert.NoError(t, err)
				assert.Equal(t, stack.Spec.Elasticsearch.Version, createdStack.Spec.Elasticsearch.Version)
			},
		},

		{
			Name: "ES pods should be created",
			Test: helpers.Eventually(func() error {
				fmt.Print(".") // progress bar 2.0
				return k.CheckPodCount(helpers.ESPodListOptions(stack.Name), int(stack.Spec.Elasticsearch.NodeCount()))
			}),
		},

		{
			Name: "Kibana pods should be created",
			Test: helpers.Eventually(func() error {
				fmt.Print(".") // progress bar 2.0
				return k.CheckPodCount(helpers.KibanaPodListOptions(stack.Name), int(stack.Spec.Kibana.NodeCount))
			}),
		},

		{
			Name: "Services should be created",
			Test: helpers.Eventually(func() error {
				for _, s := range []string{
					stack.Name + "-es-discovery",
					stack.Name + "-es-public",
					stack.Name + "-kibana",
				} {
					if _, err := k.GetService(s); err != nil {
						return err
					}
				}
				return nil
			}),
		},

		{
			Name: "ES pods should Eventually be in a Running state",
			Test: helpers.Eventually(func() error {
				pods, err := k.GetPods(helpers.ESPodListOptions(stack.Name))
				if err != nil {
					return err
				}
				for _, p := range pods {
					if p.Status.Phase != corev1.PodRunning {
						return fmt.Errorf("Pod not running yet")
					}
				}
				return nil
			}),
		},

		{
			Name: "ES pods should Eventually be Ready",
			Test: helpers.Eventually(func() error {
				pods, err := k.GetPods(helpers.ESPodListOptions(stack.Name))
				if err != nil {
					return err
				}
				for _, p := range pods {
					statuses := p.Status.ContainerStatuses
					if statuses == nil || len(statuses) == 0 {
						return fmt.Errorf("No container status set yet")
					}
					if !statuses[0].Ready {
						return fmt.Errorf("Container not ready yet")
					}
				}
				return nil
			}),
		},

		{
			Name: "Services should have endpoints",
			Test: helpers.Eventually(func() error {
				for endpointName, addrCount := range map[string]int{
					stack.Name + "-es-discovery": int(stack.Spec.Elasticsearch.NodeCount()),
					stack.Name + "-kibana":       int(stack.Spec.Kibana.NodeCount),
					stack.Name + "-es-public":    int(stack.Spec.Elasticsearch.NodeCount()),
				} {
					endpoints, err := k.GetEndpoints(endpointName)
					if err != nil {
						return err
					}
					if len(endpoints.Subsets) == 0 {
						return fmt.Errorf("No subset for endpoint %s", endpointName)
					}
					if len(endpoints.Subsets[0].Addresses) != addrCount {
						fmt.Println(endpoints.Subsets)
						return fmt.Errorf("%d addresses found for endpoint %s, expected %d", len(endpoints.Subsets[0].Addresses), endpointName, addrCount)
					}
				}
				return nil
			}),
		},

		{
			Name: "Elastic password should be available",
			Test: func(t *testing.T) {
				password, err := k.GetElasticPassword(stack.Name)
				assert.NoError(t, err)
				assert.NotEqual(t, "", password)
			},
		},
	}
}

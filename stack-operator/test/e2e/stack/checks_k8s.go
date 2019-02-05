package stack

import (
	"fmt"
	"testing"

	"github.com/elastic/k8s-operators/stack-operator/pkg/apis/deployments/v1alpha1"
	estype "github.com/elastic/k8s-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/stack-operator/test/e2e/helpers"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

// K8sStackChecks returns all test steps to verify the given stack
// in K8s is the expected one
func K8sStackChecks(stack v1alpha1.Stack, k8sClient *helpers.K8sHelper) helpers.TestStepList {
	return helpers.TestStepList{
		CheckKibanaDeployment(stack, k8sClient),
		CheckKibanaPodsCount(stack, k8sClient),
		CheckESVersion(stack, k8sClient),
		CheckKibanaPodsRunning(stack, k8sClient),
		CheckESPodsRunning(stack, k8sClient),
		CheckServices(stack, k8sClient),
		CheckESPodsReady(stack, k8sClient),
		CheckESPodsResources(stack, k8sClient),
		CheckServicesEndpoints(stack, k8sClient),
		CheckClusterHealth(stack, k8sClient),
		CheckClusterUUID(stack, k8sClient),
		CheckESPassword(stack, k8sClient),
	}
}

// CheckKibanaDeployment checks that Kibana deployment exists
func CheckKibanaDeployment(stack v1alpha1.Stack, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "Kibana deployment should be set",
		Test: helpers.Eventually(func() error {
			var dep appsv1.Deployment
			err := k.Client.Get(types.NamespacedName{
				Namespace: helpers.DefaultNamespace,
				Name:      stack.Name + "-kibana",
			}, &dep)
			if stack.Spec.Kibana.NodeCount == 0 && apierrors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if *dep.Spec.Replicas != stack.Spec.Kibana.NodeCount {
				return fmt.Errorf("invalid Kibana replicas count: expected %d, got %d", stack.Spec.Kibana.NodeCount, *dep.Spec.Replicas)
			}
			return nil
		}),
	}
}

// CheckKibanaPodsCount checks that Kibana pods count matches the expected one
func CheckKibanaPodsCount(stack v1alpha1.Stack, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "Kibana pods count should match the expected one",
		Test: helpers.Eventually(func() error {
			return k.CheckPodCount(helpers.KibanaPodListOptions(stack.Name), int(stack.Spec.Kibana.NodeCount))
		}),
	}
}

// CheckESPodsRunning checks that all ES pods for the given stack are running
func CheckESPodsRunning(stack v1alpha1.Stack, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "ES pods should eventually be running",
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
	}
}

// CheckKibanaPodsRunning checks that all ES pods for the given stack are running
func CheckKibanaPodsRunning(stack v1alpha1.Stack, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "Kibana pods should eventually be running",
		Test: helpers.Eventually(func() error {
			pods, err := k.GetPods(helpers.KibanaPodListOptions(stack.Name))
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
	}
}

// CheckESVersion checks that the running ES version is the expected one
// TODO: request ES endpoint instead, not the k8s implementation detail
func CheckESVersion(stack v1alpha1.Stack, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "ES version should be the expected one",
		Test: helpers.Eventually(func() error {
			pods, err := k.GetPods(helpers.ESPodListOptions(stack.Name))
			if err != nil {
				return err
			}
			// check number of pods
			if len(pods) != int(stack.Spec.Elasticsearch.NodeCount()) {
				return fmt.Errorf("Expected %d pods, got %d", stack.Spec.Elasticsearch.NodeCount(), len(pods))
			}
			// check ES version label
			for _, p := range pods {
				version := p.Labels["elasticsearch.stack.k8s.elastic.co/version"]
				if version != stack.Spec.Version {
					return fmt.Errorf("Version %s does not match expected version %s", stack.Spec.Version, version)
				}
			}
			return nil
		}),
	}
}

// CheckESPodsReady retrieves ES pods from the given stack,
// and check they are in status ready, until success
func CheckESPodsReady(stack v1alpha1.Stack, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "Pods should eventually be ready",
		Test: helpers.Eventually(func() error {
			pods, err := k.GetPods(helpers.ESPodListOptions(stack.Name))
			if err != nil {
				return err
			}
			// check number of pods
			if len(pods) != int(stack.Spec.Elasticsearch.NodeCount()) {
				return fmt.Errorf("Expected %d pods, got %d", stack.Spec.Elasticsearch.NodeCount(), len(pods))
			}
			// check pod statuses
		podsLoop:
			for _, p := range pods {
				for _, c := range p.Status.Conditions {
					if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
						// pod is ready, move on to next pod
						continue podsLoop
					}
				}
				return fmt.Errorf("Pod %s is not ready yet", p.Name)
			}
			return nil
		}),
	}
}

type mytesting struct {
	hasErr bool
}

func (m mytesting) Errorf(s string, args ...interface{}) {
	m.hasErr = true
}

// CheckClusterHealth checks that the given stack status reports a green ES health
func CheckClusterHealth(stack v1alpha1.Stack, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "Cluster health should eventually be green",
		Test: helpers.Eventually(func() error {
			var stackRes v1alpha1.Stack
			err := k.Client.Get(GetNamespacedName(stack), &stackRes)
			if err != nil {
				return err
			}
			if stackRes.Status.Elasticsearch.Health != estype.ElasticsearchGreenHealth {
				return fmt.Errorf("Health is %s", stackRes.Status.Elasticsearch.Health)
			}
			return nil
		}),
	}
}

// CheckESPodsResources checks that ES pods from the given stack have the expected resources
// TODO: request Elasticsearch endpoint, to also validate what's seen from ES
func CheckESPodsResources(stack v1alpha1.Stack, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "Pods should eventually have the expected resources",
		Test: helpers.Eventually(func() error {
			pods, err := k.GetPods(helpers.ESPodListOptions(stack.Name))
			if err != nil {
				return err
			}
			var expectedLimits []corev1.ResourceList
			for _, topo := range stack.Spec.Elasticsearch.Topologies {
				for i := 0; i < int(topo.NodeCount); i++ {
					expectedLimits = append(expectedLimits, topo.Resources.Limits)
				}
			}
			var limits []corev1.ResourceList
			for _, p := range pods {
				if len(p.Spec.Containers) == 0 {
					return fmt.Errorf("No ES container found in pod %s", p.Name)
				}
				esContainer := p.Spec.Containers[0]
				limits = append(limits, esContainer.Resources.Limits)
			}
			if err := helpers.ElementsMatch(expectedLimits, limits); err != nil {
				return err
			}
			return nil
		}),
	}
}

// CheckServices checks that all stack services are created
func CheckServices(stack v1alpha1.Stack, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
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
	}
}

// CheckServicesEndpoints checks that services have the expected number of endpoints
func CheckServicesEndpoints(stack v1alpha1.Stack, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "Services should have endpoints",
		Test: helpers.Eventually(func() error {
			for endpointName, addrCount := range map[string]int{
				stack.Name + "-es-discovery": int(stack.Spec.Elasticsearch.NodeCount()),
				stack.Name + "-kibana":       int(stack.Spec.Kibana.NodeCount),
				stack.Name + "-es-public":    int(stack.Spec.Elasticsearch.NodeCount()),
			} {
				if addrCount == 0 {
					continue // maybe no Kibana in this stack
				}
				endpoints, err := k.GetEndpoints(endpointName)
				if err != nil {
					return err
				}
				if len(endpoints.Subsets) == 0 {
					return fmt.Errorf("No subset for endpoint %s", endpointName)
				}
				if len(endpoints.Subsets[0].Addresses) != addrCount {
					return fmt.Errorf("%d addresses found for endpoint %s, expected %d", len(endpoints.Subsets[0].Addresses), endpointName, addrCount)
				}
			}
			return nil
		}),
	}
}

// CheckClusterUUID checks that the cluster ID is eventually set in the stack status
func CheckClusterUUID(stack v1alpha1.Stack, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "Cluster UUID should eventually appear in the stack status",
		Test: helpers.Eventually(func() error {
			var s v1alpha1.Stack
			err := k.Client.Get(GetNamespacedName(stack), &s)
			if err != nil {
				return err
			}
			if s.Status.Elasticsearch.ClusterUUID == "" {
				return fmt.Errorf("ClusterUUID not set")
			}
			return nil
		}),
	}
}

// CheckESPassword checks that the user password to access ES is correctly set
func CheckESPassword(stack v1alpha1.Stack, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "Elastic password should be available",
		Test: func(t *testing.T) {
			password, err := k.GetElasticPassword(stack.Name)
			require.NoError(t, err)
			require.NotEqual(t, "", password)
		},
	}
}

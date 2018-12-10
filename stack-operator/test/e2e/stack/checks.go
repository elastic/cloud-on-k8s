package stack

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/elastic/stack-operators/stack-operator/test/e2e/helpers"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/deployments/v1alpha1"
	estype "github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
)

// CheckStackSteps returns all test steps to verify the given stack
// in K8s is the expected one
func CheckStackSteps(stack v1alpha1.Stack, k *helpers.K8sHelper) []helpers.TestStep {
	return []helpers.TestStep{
		CheckKibanaPodsCount(stack, k),
		CheckTopology(stack, k),
		CheckESVersion(stack, k),
		CheckKibanaPodsRunning(stack, k),
		CheckESPodsRunning(stack, k),
		CheckServices(stack, k),
		CheckESPodsReady(stack, k),
		CheckESPodsResources(stack, k),
		CheckServicesEndpoints(stack, k),
		CheckClusterHealth(stack, k),
		CheckClusterUUID(stack, k),
		CheckESPassword(stack, k),
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

// CheckTopology checks that the pods in K8S match
// the topology specified in the stack
// TODO: request Elasticsearch /nodes endpoint instead, to move away from implementation details
func CheckTopology(stack v1alpha1.Stack, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "Pods should have the expected topology",
		Test: helpers.Eventually(func() error {
			pods, err := k.GetPods(helpers.ESPodListOptions(stack.Name))
			if err != nil {
				return err
			}
			if len(pods) != int(stack.Spec.Elasticsearch.NodeCount()) {
				return fmt.Errorf("Actual number of pods %d does not match expected %d", len(pods), int(stack.Spec.Elasticsearch.NodeCount()))
			}
			// compare expected vs. actual node types
			nodesTypes, err := extractNodesTypes(pods)
			if err != nil {
				return err
			}
			expectedNodesTypes := []estype.NodeTypesSpec{}
			for _, topo := range stack.Spec.Elasticsearch.Topologies {
				for i := 0; i < int(topo.NodeCount); i++ {
					expectedNodesTypes = append(expectedNodesTypes, topo.NodeTypes)
				}
			}
			if err := helpers.CheckSameUnorderedElements(expectedNodesTypes, nodesTypes); err != nil {
				return err
			}
			return nil
		}),
	}
}

// CheckClusterHealth checks that the given stack status reports a green ES health
func CheckClusterHealth(stack v1alpha1.Stack, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "Cluster health should eventually be green",
		Test: helpers.Eventually(func() error {
			var stackRes v1alpha1.Stack
			err := k.Client.Get(helpers.DefaultCtx, GetNamespacedName(stack), &stackRes)
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
			if err := helpers.CheckSameUnorderedElements(expectedLimits, limits); err != nil {
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
			err := k.Client.Get(helpers.DefaultCtx, GetNamespacedName(stack), &s)
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
			assert.NoError(t, err)
			assert.NotEqual(t, "", password)
		},
	}
}

// extractNodesTypes parses NodeTypesSpec in the given list of pods
func extractNodesTypes(pods []corev1.Pod) ([]estype.NodeTypesSpec, error) {
	var nodesTypes []estype.NodeTypesSpec
	for _, p := range pods {
		if len(p.Spec.Containers) != 1 {
			return nil, fmt.Errorf("More than 1 container in pod %s", p.Name)
		}
		esContainer := p.Spec.Containers[0]
		env := map[string]string{}
		for _, envVar := range esContainer.Env {
			switch envVar.Name {
			case "node.master", "node.data", "node.ingest", "node.ml":
				env[envVar.Name] = envVar.Value
			}
		}
		if len(env) != 4 {
			return nil, fmt.Errorf("Expected node topologies env var to be set, got %+v", env)
		}
		isMaster, err := strconv.ParseBool(env["node.master"])
		if err != nil {
			return nil, err
		}
		isData, err := strconv.ParseBool(env["node.data"])
		if err != nil {
			return nil, err
		}
		isIngest, err := strconv.ParseBool(env["node.ingest"])
		if err != nil {
			return nil, err
		}
		isML, err := strconv.ParseBool(env["node.ml"])
		if err != nil {
			return nil, err
		}
		nodeTypes := estype.NodeTypesSpec{
			Master: isMaster,
			Data:   isData,
			Ingest: isIngest,
			ML:     isML,
		}
		nodesTypes = append(nodesTypes, nodeTypes)
	}
	return nodesTypes, nil
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apm

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/params"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

// K8sStackChecks returns all test steps to verify the given stack
// in K8s is the expected one
func K8sStackChecks(stack Builder, k8sClient *helpers.K8sHelper) helpers.TestStepList {
	return helpers.TestStepList{
		CheckApmServerDeployment(stack, k8sClient),
		CheckApmServerPodsCount(stack, k8sClient),
		CheckApmServerPodsRunning(stack, k8sClient),
		CheckServices(stack, k8sClient),
		CheckServicesEndpoints(stack, k8sClient),
	}
}

// CheckApmServerDeployment checks that Apm Server deployment exists
func CheckApmServerDeployment(stack Builder, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "ApmServer deployment should be created",
		Test: helpers.Eventually(func() error {
			var dep appsv1.Deployment
			err := k.Client.Get(types.NamespacedName{
				Namespace: params.Namespace,
				Name:      stack.ApmServer.Name + "-apm",
			}, &dep)
			if stack.ApmServer.Spec.NodeCount == 0 && apierrors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if *dep.Spec.Replicas != stack.ApmServer.Spec.NodeCount {
				return fmt.Errorf("invalid ApmServer replicas count: expected %d, got %d", stack.ApmServer.Spec.NodeCount, *dep.Spec.Replicas)
			}
			return nil
		}),
	}
}

// CheckApmServerPodsCount checks that Apm Server pods count matches the expected one
func CheckApmServerPodsCount(stack Builder, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "ApmServer pods count should match the expected one",
		Test: helpers.Eventually(func() error {
			return k.CheckPodCount(helpers.ApmServerPodListOptions(stack.ApmServer.Name), int(stack.ApmServer.Spec.NodeCount))
		}),
	}
}

// CheckKibanaPodsRunning checks that all ApmServer pods for the given stack are running
func CheckApmServerPodsRunning(stack Builder, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "ApmServer pods should eventually be running",
		Test: helpers.Eventually(func() error {
			pods, err := k.GetPods(helpers.ApmServerPodListOptions(stack.ApmServer.Name))
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

// CheckServices checks that all stack services are created
func CheckServices(stack Builder, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "Services should be created",
		Test: helpers.Eventually(func() error {
			for _, s := range []string{
				stack.ApmServer.Name + "-apm-http",
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
func CheckServicesEndpoints(stack Builder, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "Services should have endpoints",
		Test: helpers.Eventually(func() error {
			for endpointName, addrCount := range map[string]int{
				stack.ApmServer.Name + "-apm-http": int(stack.ApmServer.Spec.NodeCount),
			} {
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

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserver

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

func (b Builder) CheckK8sTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		CheckApmServerDeployment(b, k),
		CheckApmServerPodsCount(b, k),
		CheckApmServerPodsRunning(b, k),
		CheckServices(b, k),
		CheckServicesEndpoints(b, k),
	}
}

// CheckApmServerDeployment checks that APM Server deployment exists
func CheckApmServerDeployment(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "ApmServer deployment should be created",
		Test: test.Eventually(func() error {
			var dep appsv1.Deployment
			err := k.Client.Get(types.NamespacedName{
				Namespace: test.Ctx().ManagedNamespace(0),
				Name:      b.ApmServer.Name + "-apm-server",
			}, &dep)
			if b.ApmServer.Spec.NodeCount == 0 && apierrors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if *dep.Spec.Replicas != b.ApmServer.Spec.NodeCount {
				return fmt.Errorf("invalid ApmServer replicas count: expected %d, got %d", b.ApmServer.Spec.NodeCount, *dep.Spec.Replicas)
			}
			return nil
		}),
	}
}

// CheckApmServerPodsCount checks that APM Server pods count matches the expected one
func CheckApmServerPodsCount(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "ApmServer pods count should match the expected one",
		Test: test.Eventually(func() error {
			return k.CheckPodCount(test.ApmServerPodListOptions(b.ApmServer.Namespace, b.ApmServer.Name), int(b.ApmServer.Spec.NodeCount))
		}),
	}
}

// CheckApmServerPodsRunning checks that all APM Server pods for the given APM Server are running
func CheckApmServerPodsRunning(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "ApmServer pods should eventually be running",
		Test: test.Eventually(func() error {
			pods, err := k.GetPods(test.ApmServerPodListOptions(b.ApmServer.Namespace, b.ApmServer.Name))
			if err != nil {
				return err
			}
			for _, p := range pods {
				if p.Status.Phase != corev1.PodRunning {
					return fmt.Errorf("pod not running yet")
				}
			}
			return nil
		}),
	}
}

// CheckServices checks that all APM Server services are created
func CheckServices(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "ApmServer services should be created",
		Test: test.Eventually(func() error {
			for _, s := range []string{
				b.ApmServer.Name + "-apm-http",
			} {
				if _, err := k.GetService(b.ApmServer.Namespace, s); err != nil {
					return err
				}
			}
			return nil
		}),
	}
}

// CheckServicesEndpoints checks that APM Server services have the expected number of endpoints
func CheckServicesEndpoints(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "ApmServer services should have endpoints",
		Test: test.Eventually(func() error {
			for endpointName, addrCount := range map[string]int{
				b.ApmServer.Name + "-apm-http": int(b.ApmServer.Spec.NodeCount),
			} {
				endpoints, err := k.GetEndpoints(b.ApmServer.Namespace, endpointName)
				if err != nil {
					return err
				}
				if len(endpoints.Subsets) == 0 {
					return fmt.Errorf("no subset for endpoint %s", endpointName)
				}
				if len(endpoints.Subsets[0].Addresses) != addrCount {
					return fmt.Errorf("%d addresses found for endpoint %s, expected %d", len(endpoints.Subsets[0].Addresses), endpointName, addrCount)
				}
			}
			return nil
		}),
	}
}

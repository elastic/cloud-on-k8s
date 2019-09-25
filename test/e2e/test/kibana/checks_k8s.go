// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"fmt"

	kbname "github.com/elastic/cloud-on-k8s/pkg/controller/kibana/name"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

func (b Builder) CheckK8sTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		CheckKibanaDeployment(b, k),
		CheckKibanaPodsCount(b, k),
		CheckKibanaPodsRunning(b, k),
		CheckServices(b, k),
		CheckServicesEndpoints(b, k),
	}
}

// CheckKibanaDeployment checks that Kibana deployment exists
func CheckKibanaDeployment(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "Kibana deployment should be set",
		Test: test.Eventually(func() error {
			var dep appsv1.Deployment
			err := k.Client.Get(types.NamespacedName{
				Namespace: b.Kibana.Namespace,
				Name:      kbname.Deployment(b.Kibana.Name),
			}, &dep)
			if b.Kibana.Spec.NodeCount == 0 && apierrors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if *dep.Spec.Replicas != b.Kibana.Spec.NodeCount {
				return fmt.Errorf("invalid Kibana replicas count: expected %d, got %d", b.Kibana.Spec.NodeCount, *dep.Spec.Replicas)
			}
			return nil
		}),
	}
}

// CheckKibanaPodsCount checks that Kibana pods count matches the expected one
func CheckKibanaPodsCount(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "Kibana pods count should match the expected one",
		Test: test.Eventually(func() error {
			return k.CheckPodCount(int(b.Kibana.Spec.NodeCount), test.KibanaPodListOptions(b.Kibana.Namespace, b.Kibana.Name)...)
		}),
	}
}

// CheckKibanaPodsRunning checks that all Kibana pods for the given Kibana are running
func CheckKibanaPodsRunning(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "Kibana pods should eventually be running",
		Test: test.Eventually(func() error {
			pods, err := k.GetPods(test.KibanaPodListOptions(b.Kibana.Namespace, b.Kibana.Name)...)
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

// CheckServices checks that all Kibana services are created
func CheckServices(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "Kibana services should be created",
		Test: test.Eventually(func() error {
			for _, s := range []string{
				kbname.HTTPService(b.Kibana.Name),
			} {
				if _, err := k.GetService(b.Kibana.Namespace, s); err != nil {
					return err
				}
			}
			return nil
		}),
	}
}

// CheckServicesEndpoints checks that services have the expected number of endpoints
func CheckServicesEndpoints(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "Kibana services should have endpoints",
		Test: test.Eventually(func() error {
			for endpointName, addrCount := range map[string]int{
				kbname.HTTPService(b.Kibana.Name): int(b.Kibana.Spec.NodeCount),
			} {
				if addrCount == 0 {
					continue // maybe no Kibana in this b
				}
				endpoints, err := k.GetEndpoints(b.Kibana.Namespace, endpointName)
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

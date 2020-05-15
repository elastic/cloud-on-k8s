// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserver

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/test/e2e/test"
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
		CheckSecrets(b, k),
	}
}

// CheckApmServerDeployment checks that APM Server deployment exists
func CheckApmServerDeployment(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "ApmServer deployment should be created",
		Test: test.Eventually(func() error {
			var dep appsv1.Deployment
			err := k.Client.Get(types.NamespacedName{
				Namespace: b.ApmServer.Namespace,
				Name:      b.ApmServer.Name + "-apm-server",
			}, &dep)
			if b.ApmServer.Spec.Count == 0 && apierrors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if *dep.Spec.Replicas != b.ApmServer.Spec.Count {
				return fmt.Errorf("invalid ApmServer replicas count: expected %d, got %d", b.ApmServer.Spec.Count, *dep.Spec.Replicas)
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
			return k.CheckPodCount(int(b.ApmServer.Spec.Count), test.ApmServerPodListOptions(b.ApmServer.Namespace, b.ApmServer.Name)...)
		}),
	}
}

// CheckApmServerPodsRunning checks that all APM Server pods for the given APM Server are running
func CheckApmServerPodsRunning(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "ApmServer pods should eventually be running",
		Test: test.Eventually(func() error {
			pods, err := k.GetPods(test.ApmServerPodListOptions(b.ApmServer.Namespace, b.ApmServer.Name)...)
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
				b.ApmServer.Name + "-apm-http": int(b.ApmServer.Spec.Count),
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

// CheckSecrets checks that expected secrets have been created.
func CheckSecrets(b Builder, k *test.K8sClient) test.Step {
	return test.CheckSecretsContent(k, b.ApmServer.Namespace, func() map[string][]string {
		apmName := b.ApmServer.Name
		// hardcode all secret names and keys to catch any breaking change
		expectedSecrets := map[string][]string{
			apmName + "-apm-config": {"apm-server.yml"},
			apmName + "-apm-token":  {"secret-token"},
		}
		if b.ApmServer.Spec.ElasticsearchRef.Name != "" {
			expectedSecrets[apmName+"-apm-es-ca"] = []string{"ca.crt", "tls.crt"}
			expectedSecrets[apmName+"-apm-user"] = []string{b.ApmServer.Namespace + "-" + apmName + "-apm-user"}
		}
		if b.ApmServer.Spec.HTTP.TLS.Enabled() {
			expectedSecrets[apmName+"-apm-http-ca-internal"] = []string{"tls.crt", "tls.key"}
			expectedSecrets[apmName+"-apm-http-certs-internal"] = []string{"tls.crt", "tls.key", "ca.crt"}
			expectedSecrets[apmName+"-apm-http-certs-public"] = []string{"ca.crt", "tls.crt"}
		}
		return expectedSecrets
	})
}

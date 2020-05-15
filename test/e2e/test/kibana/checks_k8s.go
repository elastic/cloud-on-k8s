// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

func (b Builder) CheckK8sTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		CheckKibanaPods(b, k),
		CheckServices(b, k),
		CheckServicesEndpoints(b, k),
		CheckSecrets(b, k),
	}
}

// CheckKibanaPods checks Kibana pods for correct builder hash, pod count, whether pods are running and ready
func CheckKibanaPods(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "Kibana deployment be rolled out",
		Test: test.Eventually(func() error {
			var dep appsv1.Deployment
			err := k.Client.Get(types.NamespacedName{
				Namespace: b.Kibana.Namespace,
				Name:      kibana.Deployment(b.Kibana.Name),
			}, &dep)
			if b.Kibana.Spec.Count == 0 && apierrors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}

			var pods corev1.PodList
			if err := k.Client.List(&pods, test.KibanaPodListOptions(b.Kibana.Namespace, b.Kibana.Name)...); err != nil {
				return err
			}

			// builder hash matches
			goodBuilderHash := hash.HashObject(b.Kibana.Spec)
			for _, pod := range pods.Items {
				if err := test.ValidateBuilderHashAnnotation(pod, goodBuilderHash); err != nil {
					return err
				}
			}

			// pod count matches
			if len(pods.Items) != int(b.Kibana.Spec.Count) {
				return fmt.Errorf("invalid Kibana pod count: expected %d, got %d", b.Kibana.Spec.Count, len(pods.Items))
			}

			// pods are running
			for _, pod := range pods.Items {
				if pod.Status.Phase != corev1.PodRunning {
					return fmt.Errorf("pod not running yet")
				}
			}

			// pods are ready
			for _, pod := range pods.Items {
				if !k8s.IsPodReady(pod) {
					return fmt.Errorf("pod not ready yet")
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
				b.Kibana.Name + "-kb-http",
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
				b.Kibana.Name + "-kb-http": int(b.Kibana.Spec.Count),
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

// CheckSecrets checks that expected secrets have been created.
func CheckSecrets(b Builder, k *test.K8sClient) test.Step {
	return test.CheckSecretsContent(k, b.Kibana.Namespace, func() map[string][]string {
		kbName := b.Kibana.Name
		// hardcode all secret names and keys to catch any breaking change
		expectedSecrets := map[string][]string{
			kbName + "-kb-config": {"kibana.yml", "telemetry.yml"},
		}
		if b.Kibana.Spec.ElasticsearchRef.Name != "" {
			expectedSecrets[kbName+"-kb-es-ca"] = []string{"ca.crt", "tls.crt"}
			expectedSecrets[kbName+"-kibana-user"] = []string{b.Kibana.Namespace + "-" + kbName + "-kibana-user"}
		}
		if b.Kibana.Spec.HTTP.TLS.Enabled() {
			expectedSecrets[kbName+"-kb-http-ca-internal"] = []string{"tls.crt", "tls.key"}
			expectedSecrets[kbName+"-kb-http-certs-internal"] = []string{"tls.crt", "tls.key", "ca.crt"}
			expectedSecrets[kbName+"-kb-http-certs-public"] = []string{"ca.crt", "tls.crt"}
		}
		return expectedSecrets
	})
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stack

import (
	"fmt"

	estype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	esname "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	kbname "github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
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
		CheckCertificateAuthority(stack, k8sClient),
		CheckKibanaDeployment(stack, k8sClient),
		CheckKibanaPodsCount(stack, k8sClient),
		CheckESVersion(stack, k8sClient),
		CheckKibanaPodsRunning(stack, k8sClient),
		CheckESPodsRunning(stack, k8sClient),
		CheckServices(stack, k8sClient),
		CheckESPodsReady(stack, k8sClient),
		CheckPodCertificates(stack, k8sClient),
		CheckServicesEndpoints(stack, k8sClient),
		CheckClusterHealth(stack, k8sClient),
		CheckClusterUUID(stack, k8sClient),
		CheckESPassword(stack, k8sClient),
		CheckESDataVolumeType(stack.Elasticsearch, k8sClient),
	}
}

// CheckCertificateAuthority checks that the CA is fully setup (CA cert + private key)
func CheckCertificateAuthority(stack Builder, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "Certificate Authority should be set and deployed",
		Test: helpers.Eventually(func() error {
			// Check that the Transport CA may be loaded
			_, err := k.GetCA(stack.Elasticsearch.Name, certificates.TransportCAType)
			if err != nil {
				return err
			}

			// Check that the HTTP CA may be loaded
			_, err = k.GetCA(stack.Elasticsearch.Name, certificates.HTTPCAType)
			if err != nil {
				return err
			}

			return nil
		}),
	}
}

// CheckPodCertificates checks that all pods have a private key and signed certificate
func CheckPodCertificates(stack Builder, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "Every pod should have a certificate",
		Test: helpers.Eventually(func() error {
			pods, err := k.GetPods(helpers.ESPodListOptions(stack.Elasticsearch.Name))
			if err != nil {
				return err
			}
			for _, pod := range pods {
				_, _, err := k.GetTransportCert(pod.Name)
				if err != nil {
					return err
				}
			}
			return nil
		}),
	}
}

// CheckKibanaDeployment checks that Kibana deployment exists
func CheckKibanaDeployment(stack Builder, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "Kibana deployment should be set",
		Test: helpers.Eventually(func() error {
			var dep appsv1.Deployment
			err := k.Client.Get(types.NamespacedName{
				Namespace: params.Namespace,
				Name:      kbname.Deployment(stack.Kibana.Name),
			}, &dep)
			if stack.Kibana.Spec.NodeCount == 0 && apierrors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if *dep.Spec.Replicas != stack.Kibana.Spec.NodeCount {
				return fmt.Errorf("invalid Kibana replicas count: expected %d, got %d", stack.Kibana.Spec.NodeCount, *dep.Spec.Replicas)
			}
			return nil
		}),
	}
}

// CheckKibanaPodsCount checks that Kibana pods count matches the expected one
func CheckKibanaPodsCount(stack Builder, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "Kibana pods count should match the expected one",
		Test: helpers.Eventually(func() error {
			return k.CheckPodCount(helpers.KibanaPodListOptions(stack.Kibana.Name), int(stack.Kibana.Spec.NodeCount))
		}),
	}
}

// CheckESPodsRunning checks that all ES pods for the given stack are running
func CheckESPodsRunning(stack Builder, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "ES pods should eventually be running",
		Test: helpers.Eventually(func() error {
			pods, err := k.GetPods(helpers.ESPodListOptions(stack.Elasticsearch.Name))
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
func CheckKibanaPodsRunning(stack Builder, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "Kibana pods should eventually be running",
		Test: helpers.Eventually(func() error {
			pods, err := k.GetPods(helpers.KibanaPodListOptions(stack.Kibana.Name))
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
func CheckESVersion(stack Builder, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "ES version should be the expected one",
		Test: helpers.Eventually(func() error {
			pods, err := k.GetPods(helpers.ESPodListOptions(stack.Elasticsearch.Name))
			if err != nil {
				return err
			}
			// check number of pods
			if len(pods) != int(stack.Elasticsearch.Spec.NodeCount()) {
				return fmt.Errorf("Expected %d pods, got %d", stack.Elasticsearch.Spec.NodeCount(), len(pods))
			}
			// check ES version label
			for _, p := range pods {
				version := p.Labels[label.VersionLabelName]
				if version != stack.Elasticsearch.Spec.Version {
					return fmt.Errorf("Version %s does not match expected version %s", version, stack.Elasticsearch.Spec.Version)
				}
			}
			return nil
		}),
	}
}

// CheckESPodsReady retrieves ES pods from the given stack,
// and check they are in status ready, until success
func CheckESPodsReady(stack Builder, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "Pods should eventually be ready",
		Test: helpers.Eventually(func() error {
			pods, err := k.GetPods(helpers.ESPodListOptions(stack.Elasticsearch.Name))
			if err != nil {
				return err
			}
			// check number of pods
			if len(pods) != int(stack.Elasticsearch.Spec.NodeCount()) {
				return fmt.Errorf("Expected %d pods, got %d", stack.Elasticsearch.Spec.NodeCount(), len(pods))
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
func CheckClusterHealth(stack Builder, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "Cluster health should eventually be green",
		Test: helpers.Eventually(func() error {
			var es estype.Elasticsearch
			err := k.Client.Get(k8s.ExtractNamespacedName(&stack.Elasticsearch), &es)
			if err != nil {
				return err
			}
			if es.Status.Health != estype.ElasticsearchGreenHealth {
				return fmt.Errorf("Health is %s", es.Status.Health)
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
				esname.HTTPService(stack.Elasticsearch.Name),
				kbname.HTTPService(stack.Kibana.Name),
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
				kbname.HTTPService(stack.Kibana.Name):        int(stack.Kibana.Spec.NodeCount),
				esname.HTTPService(stack.Elasticsearch.Name): int(stack.Elasticsearch.Spec.NodeCount()),
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
func CheckClusterUUID(stack Builder, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "Cluster UUID should eventually appear in the stack status",
		Test: helpers.Eventually(func() error {
			var es estype.Elasticsearch
			err := k.Client.Get(k8s.ExtractNamespacedName(&stack.Elasticsearch), &es)
			if err != nil {
				return err
			}
			if es.Status.ClusterUUID == "" {
				return fmt.Errorf("ClusterUUID not set")
			}
			return nil
		}),
	}
}

// CheckESPassword checks that the user password to access ES is correctly set
func CheckESPassword(stack Builder, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "Elastic password should be available",
		Test: helpers.Eventually(func() error {
			password, err := k.GetElasticPassword(stack.Elasticsearch.Name)
			if err != nil {
				return err
			}
			if password == "" {
				return fmt.Errorf("user password is not set")
			}
			return nil
		}),
	}
}

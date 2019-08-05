// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"fmt"

	estype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	esname "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test"
	corev1 "k8s.io/api/core/v1"
)

func (b Builder) CheckK8sTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		CheckCertificateAuthority(b, k),
		CheckESVersion(b, k),
		CheckESPodsRunning(b, k),
		CheckServices(b, k),
		CheckESPodsReady(b, k),
		CheckPodCertificates(b, k),
		CheckServicesEndpoints(b, k),
		CheckClusterHealth(b, k),
		CheckClusterUUID(b, k),
		CheckESPassword(b, k),
		CheckESDataVolumeType(b.Elasticsearch, k),
	}
}

// CheckCertificateAuthority checks that the CA is fully setup (CA cert + private key)
func CheckCertificateAuthority(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "ES certificate authority should be set and deployed",
		Test: test.Eventually(func() error {
			// Check that the Transport CA may be loaded
			_, err := k.GetCA(b.Elasticsearch.Namespace, b.Elasticsearch.Name, certificates.TransportCAType)
			if err != nil {
				return err
			}

			// Check that the HTTP CA may be loaded
			_, err = k.GetCA(b.Elasticsearch.Namespace, b.Elasticsearch.Name, certificates.HTTPCAType)
			if err != nil {
				return err
			}

			return nil
		}),
	}
}

// CheckPodCertificates checks that all pods have a private key and signed certificate
func CheckPodCertificates(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "ES pods should eventually have a certificate",
		Test: test.Eventually(func() error {
			pods, err := k.GetPods(test.ESPodListOptions(b.Elasticsearch.Namespace, b.Elasticsearch.Name))
			if err != nil {
				return err
			}
			for _, pod := range pods {
				_, _, err := k.GetTransportCert(pod.Namespace, pod.Name)
				if err != nil {
					return err
				}
			}
			return nil
		}),
	}
}

// CheckESPodsRunning checks that all ES pods for the given ES are running
func CheckESPodsRunning(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "ES pods should eventually be running",
		Test: test.Eventually(func() error {
			pods, err := k.GetPods(test.ESPodListOptions(b.Elasticsearch.Namespace, b.Elasticsearch.Name))
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

// CheckESVersion checks that the running ES version is the expected one
// TODO: request ES endpoint instead, not the k8s implementation detail
func CheckESVersion(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "ES version should be the expected one",
		Test: test.Eventually(func() error {
			pods, err := k.GetPods(test.ESPodListOptions(b.Elasticsearch.Namespace, b.Elasticsearch.Name))
			if err != nil {
				return err
			}
			// check number of pods
			if len(pods) != int(b.Elasticsearch.Spec.NodeCount()) {
				return fmt.Errorf("expected %d pods, got %d", b.Elasticsearch.Spec.NodeCount(), len(pods))
			}
			// check ES version label
			for _, p := range pods {
				version := p.Labels[label.VersionLabelName]
				if version != b.Elasticsearch.Spec.Version {
					return fmt.Errorf("version %s does not match expected version %s", version, b.Elasticsearch.Spec.Version)
				}
			}
			return nil
		}),
	}
}

// CheckESPodsReady retrieves ES pods from the given ES,
// and check they are in status ready, until success
func CheckESPodsReady(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "ES pods should eventually be ready",
		Test: test.Eventually(func() error {
			pods, err := k.GetPods(test.ESPodListOptions(b.Elasticsearch.Namespace, b.Elasticsearch.Name))
			if err != nil {
				return err
			}
			// check number of pods
			if len(pods) != int(b.Elasticsearch.Spec.NodeCount()) {
				return fmt.Errorf("expected %d pods, got %d", b.Elasticsearch.Spec.NodeCount(), len(pods))
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
				return fmt.Errorf("pod %s is not ready yet", p.Name)
			}
			return nil
		}),
	}
}

// CheckClusterHealth checks that the given ES status reports a green ES health
func CheckClusterHealth(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "ES cluster health should eventually be green",
		Test: test.Eventually(func() error {
			var es estype.Elasticsearch
			err := k.Client.Get(k8s.ExtractNamespacedName(&b.Elasticsearch), &es)
			if err != nil {
				return err
			}
			if es.Status.Health != estype.ElasticsearchGreenHealth {
				return fmt.Errorf("health is %s", es.Status.Health)
			}
			return nil
		}),
	}
}

// CheckServices checks that all ES services are created
func CheckServices(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "ES services should be created",
		Test: test.Eventually(func() error {
			for _, s := range []string{
				esname.HTTPService(b.Elasticsearch.Name),
			} {
				if _, err := k.GetService(b.Elasticsearch.Namespace, s); err != nil {
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
		Name: "ES services should have endpoints",
		Test: test.Eventually(func() error {
			for endpointName, addrCount := range map[string]int{
				esname.HTTPService(b.Elasticsearch.Name): int(b.Elasticsearch.Spec.NodeCount()),
			} {
				if addrCount == 0 {
					continue // maybe no Kibana
				}
				endpoints, err := k.GetEndpoints(b.Elasticsearch.Namespace, endpointName)
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

// CheckClusterUUID checks that the cluster ID is eventually set in the ES status
func CheckClusterUUID(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "ES cluster UUID should eventually appear in the ES status",
		Test: test.Eventually(func() error {
			var es estype.Elasticsearch
			err := k.Client.Get(k8s.ExtractNamespacedName(&b.Elasticsearch), &es)
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
func CheckESPassword(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "Elastic password should be available",
		Test: test.Eventually(func() error {
			password, err := k.GetElasticPassword(b.Elasticsearch.Namespace, b.Elasticsearch.Name)
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

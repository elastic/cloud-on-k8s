// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserver

import (
	"context"
	"fmt"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
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
		CheckStatus(b, k),
	}
}

// CheckApmServerDeployment checks that APM Server deployment exists
func CheckApmServerDeployment(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "ApmServer deployment should be created",
		Test: test.Eventually(func() error {
			var dep appsv1.Deployment
			err := k.Client.Get(context.Background(), types.NamespacedName{
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
	return test.CheckSecretsContent(k, b.ApmServer.Namespace, func() []test.ExpectedSecret {
		apmName := b.ApmServer.Name
		apmNamespace := b.ApmServer.Namespace
		// hardcode all secret names and keys to catch any breaking change
		expected := []test.ExpectedSecret{
			{
				Name: apmName + "-apm-config",
				Keys: []string{"apm-server.yml"},
				Labels: map[string]string{
					"apm.k8s.elastic.co/name":    apmName,
					"common.k8s.elastic.co/type": "apm-server",
				},
			},
			{
				Name: apmName + "-apm-token",
				Keys: []string{"secret-token"},
				Labels: map[string]string{
					"apm.k8s.elastic.co/name":        apmName,
					"common.k8s.elastic.co/type":     "apm-server",
					"eck.k8s.elastic.co/credentials": "true",
				},
			},
		}
		if b.ApmServer.Spec.ElasticsearchRef.Name != "" {
			expected = append(expected,
				test.ExpectedSecret{
					Name: apmName + "-apm-es-ca",
					Keys: []string{"ca.crt", "tls.crt"},
					Labels: map[string]string{
						"apmassociation.k8s.elastic.co/name":        apmName,
						"apmassociation.k8s.elastic.co/namespace":   apmNamespace,
						"elasticsearch.k8s.elastic.co/cluster-name": b.ApmServer.Spec.ElasticsearchRef.Name,
					},
				},
				test.ExpectedSecret{
					Name: apmName + "-apm-user",
					Keys: []string{b.ApmServer.Namespace + "-" + apmName + "-apm-user"},
					Labels: map[string]string{
						"apmassociation.k8s.elastic.co/name":        apmName,
						"apmassociation.k8s.elastic.co/namespace":   apmNamespace,
						"eck.k8s.elastic.co/credentials":            "true",
						"elasticsearch.k8s.elastic.co/cluster-name": b.ApmServer.Spec.ElasticsearchRef.Name,
					},
				},
			)
		}
		if b.ApmServer.Spec.HTTP.TLS.Enabled() {
			expected = append(expected,
				test.ExpectedSecret{
					Name: apmName + "-apm-http-ca-internal",
					Keys: []string{"tls.crt", "tls.key"},
					Labels: map[string]string{
						"apm.k8s.elastic.co/name":    apmName,
						"common.k8s.elastic.co/type": "apm-server",
					},
				},
				test.ExpectedSecret{
					Name: apmName + "-apm-http-certs-internal",
					Keys: []string{"tls.crt", "tls.key", "ca.crt"},
					Labels: map[string]string{
						"apm.k8s.elastic.co/name":    apmName,
						"common.k8s.elastic.co/type": "apm-server",
					},
				},
				test.ExpectedSecret{
					Name: apmName + "-apm-http-certs-public",
					Keys: []string{"ca.crt", "tls.crt"},
					Labels: map[string]string{
						"apm.k8s.elastic.co/name":    apmName,
						"common.k8s.elastic.co/type": "apm-server",
					},
				},
			)
		}
		return expected
	})
}

func CheckStatus(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "APMServer status should be updated",
		Test: test.Eventually(func() error {
			var as apmv1.ApmServer
			if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.ApmServer), &as); err != nil {
				return err
			}
			// don't check association statuses that may vary across tests
			as.Status.ElasticsearchAssociationStatus = ""
			as.Status.KibanaAssociationStatus = ""

			expected := apmv1.ApmServerStatus{
				ExternalService:       b.ApmServer.Name + "-apm-http",
				SecretTokenSecretName: b.ApmServer.Name + "-apm-token",
				DeploymentStatus: commonv1.DeploymentStatus{
					AvailableNodes: b.ApmServer.Spec.Count,
					Version:        b.ApmServer.Spec.Version,
					Health:         "green",
				},
			}
			if as.Status != expected {
				return fmt.Errorf("expected status %+v but got %+v", expected, as.Status)
			}
			return nil
		}),
	}
}

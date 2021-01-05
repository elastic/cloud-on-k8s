// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package enterprisesearch

import (
	"context"
	"fmt"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

func (b Builder) CheckK8sTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		CheckDeployment(b, k),
		CheckPods(b, k),
		CheckServices(b, k),
		CheckServicesEndpoints(b, k),
		CheckSecrets(b, k),
		CheckStatus(b, k),
	}
}

// CheckDeployment checks the Deployment resource exists
func CheckDeployment(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "EnterpriseSearch deployment should be created",
		Test: test.Eventually(func() error {
			var dep appsv1.Deployment
			err := k.Client.Get(context.Background(), types.NamespacedName{
				Namespace: b.EnterpriseSearch.Namespace,
				Name:      b.EnterpriseSearch.Name + "-ent",
			}, &dep)
			if b.EnterpriseSearch.Spec.Count == 0 && apierrors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if *dep.Spec.Replicas != b.EnterpriseSearch.Spec.Count {
				return fmt.Errorf("invalid EnterpriseSearch replicas count: expected %d, got %d", b.EnterpriseSearch.Spec.Count, *dep.Spec.Replicas)
			}
			return nil
		}),
	}
}

// CheckPods checks expected Enterprise Search pods are eventually ready.
// TODO: use a common function for all deployments (kb, apm, ent)
func CheckPods(b Builder, k *test.K8sClient) test.Step {
	// It is common for Enterprise Search Pods to take some time to be ready, especially
	// during the initial bootstrap, or during version upgrades. Let's increase the timeout
	// for this particular step.
	timeout := test.Ctx().TestTimeout * 2
	return test.Step{
		Name: "Enterprise Search Pods should eventually be ready",
		Test: test.UntilSuccess(func() error {
			var pods corev1.PodList
			if err := k.Client.List(context.Background(), &pods, test.EnterpriseSearchPodListOptions(b.EnterpriseSearch.Namespace, b.EnterpriseSearch.Name)...); err != nil {
				return err
			}

			// builder hash matches
			expectedBuilderHash := hash.HashObject(b.EnterpriseSearch.Spec)
			for _, pod := range pods.Items {
				if err := test.ValidateBuilderHashAnnotation(pod, expectedBuilderHash); err != nil {
					return err
				}
			}

			// pod count matches
			if len(pods.Items) != int(b.EnterpriseSearch.Spec.Count) {
				return fmt.Errorf("invalid EnterpriseSearch pod count: expected %d, got %d", b.EnterpriseSearch.Spec.Count, len(pods.Items))
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
		}, timeout),
	}
}

// CheckServices checks that all Enterprise Search services are created
// TODO: refactor with other resources
func CheckServices(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "Enterprise Search services should be created",
		Test: test.Eventually(func() error {
			for _, s := range []string{
				b.EnterpriseSearch.Name + "-ent-http",
			} {
				if _, err := k.GetService(b.EnterpriseSearch.Namespace, s); err != nil {
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
		Name: "EnterpriseSearch services should have endpoints",
		Test: test.Eventually(func() error {
			for endpointName, addrCount := range map[string]int{
				b.EnterpriseSearch.Name + "-ent-http": int(b.EnterpriseSearch.Spec.Count),
			} {
				endpoints, err := k.GetEndpoints(b.EnterpriseSearch.Namespace, endpointName)
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
	return test.CheckSecretsContent(k, b.EnterpriseSearch.Namespace, func() []test.ExpectedSecret {
		entName := b.EnterpriseSearch.Name
		// hardcode all secret names and keys to catch any breaking change
		expected := []test.ExpectedSecret{
			{
				Name: entName + "-ent-config",
				Keys: []string{"enterprise-search.yml", "readiness-probe.sh"},
				Labels: map[string]string{
					"common.k8s.elastic.co/type":           "enterprise-search",
					"eck.k8s.elastic.co/credentials":       "true",
					"enterprisesearch.k8s.elastic.co/name": entName,
				},
			},
		}
		if b.EnterpriseSearch.Spec.ElasticsearchRef.Name != "" {
			expected = append(expected,
				test.ExpectedSecret{
					Name: entName + "-ent-es-ca",
					Keys: []string{"ca.crt", "tls.crt"},
					Labels: map[string]string{
						"elasticsearch.k8s.elastic.co/cluster-name": b.EnterpriseSearch.Spec.ElasticsearchRef.Name,
						"entassociation.k8s.elastic.co/name":        entName,
						"entassociation.k8s.elastic.co/namespace":   b.EnterpriseSearch.Namespace,
					},
				},
				test.ExpectedSecret{
					Name: entName + "-ent-user",
					Keys: []string{b.EnterpriseSearch.Namespace + "-" + entName + "-ent-user"},
					Labels: map[string]string{
						"eck.k8s.elastic.co/credentials":            "true",
						"elasticsearch.k8s.elastic.co/cluster-name": b.EnterpriseSearch.Spec.ElasticsearchRef.Name,
						"entassociation.k8s.elastic.co/name":        entName,
						"entassociation.k8s.elastic.co/namespace":   b.EnterpriseSearch.Namespace,
					},
				},
			)
		}
		if b.EnterpriseSearch.Spec.HTTP.TLS.Enabled() {
			expected = append(expected,
				test.ExpectedSecret{
					Name: entName + "-ent-http-ca-internal",
					Keys: []string{"tls.crt", "tls.key"},
					Labels: map[string]string{
						"enterprisesearch.k8s.elastic.co/name": entName,
						"common.k8s.elastic.co/type":           "enterprise-search",
					},
				},
				test.ExpectedSecret{
					Name: entName + "-ent-http-certs-internal",
					Keys: []string{"tls.crt", "tls.key", "ca.crt"},
					Labels: map[string]string{
						"enterprisesearch.k8s.elastic.co/name": entName,
						"common.k8s.elastic.co/type":           "enterprise-search",
					},
				},
				test.ExpectedSecret{
					Name: entName + "-ent-http-certs-public",
					Keys: []string{"ca.crt", "tls.crt"},
					Labels: map[string]string{
						"enterprisesearch.k8s.elastic.co/name": entName,
						"common.k8s.elastic.co/type":           "enterprise-search",
					},
				},
			)
		}
		return expected
	})
}

func CheckStatus(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "EnterpriseSearch status should be updated",
		Test: test.Eventually(func() error {
			var ent v1beta1.EnterpriseSearch
			if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.EnterpriseSearch), &ent); err != nil {
				return err
			}
			expected := v1beta1.EnterpriseSearchStatus{
				DeploymentStatus: commonv1.DeploymentStatus{
					AvailableNodes: b.EnterpriseSearch.Spec.Count,
					Version:        b.EnterpriseSearch.Spec.Version,
					Health:         "green",
				},
				ExternalService: b.EnterpriseSearch.Name + "-ent-http",
				Association:     commonv1.AssociationEstablished,
			}
			if ent.Status != expected {
				return fmt.Errorf("expected status %+v but got %+v", expected, ent.Status)
			}
			return nil
		}),
	}
}

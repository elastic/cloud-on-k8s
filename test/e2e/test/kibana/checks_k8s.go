// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"context"
	"fmt"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

func (b Builder) CheckK8sTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		CheckKibanaPods(b, k),
		CheckServices(b, k),
		CheckServicesEndpoints(b, k),
		CheckSecrets(b, k),
		CheckStatus(b, k),
	}
}

// CheckKibanaPods checks Kibana pods for correct builder hash, pod count, whether pods are running and ready
func CheckKibanaPods(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "Kibana deployment be rolled out",
		Test: test.Eventually(func() error {
			var dep appsv1.Deployment
			err := k.Client.Get(context.Background(), types.NamespacedName{
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
			if err := k.Client.List(context.Background(), &pods, test.KibanaPodListOptions(b.Kibana.Namespace, b.Kibana.Name)...); err != nil {
				return err
			}

			// builder hash matches
			expectedBuilderHash := hash.HashObject(b.Kibana.Spec)
			for _, pod := range pods.Items {
				if err := test.ValidateBuilderHashAnnotation(pod, expectedBuilderHash); err != nil {
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
	return test.CheckSecretsContent(k, b.Kibana.Namespace, func() []test.ExpectedSecret {
		kbName := b.Kibana.Name
		// hardcode all secret names and keys to catch any breaking change
		expected := []test.ExpectedSecret{
			{
				Name:         kbName + "-kb-config",
				Keys:         []string{"kibana.yml"},
				OptionalKeys: []string{"telemetry.yml"},
				Labels: map[string]string{
					"eck.k8s.elastic.co/credentials": "true",
					"kibana.k8s.elastic.co/name":     kbName,
				},
			},
		}
		if b.Kibana.Spec.ElasticsearchRef.Name != "" {
			expected = append(expected,
				test.ExpectedSecret{
					Name: kbName + "-kb-es-ca",
					Keys: []string{"ca.crt", "tls.crt"},
					Labels: map[string]string{
						"elasticsearch.k8s.elastic.co/cluster-name":  b.Kibana.Spec.ElasticsearchRef.Name,
						"kibanaassociation.k8s.elastic.co/name":      kbName,
						"kibanaassociation.k8s.elastic.co/namespace": b.Kibana.Namespace,
					},
				},
				test.ExpectedSecret{
					Name: kbName + "-kibana-user",
					Keys: []string{b.Kibana.Namespace + "-" + kbName + "-kibana-user"},
					Labels: map[string]string{
						"eck.k8s.elastic.co/credentials":             "true",
						"elasticsearch.k8s.elastic.co/cluster-name":  b.Kibana.Spec.ElasticsearchRef.Name,
						"kibanaassociation.k8s.elastic.co/name":      kbName,
						"kibanaassociation.k8s.elastic.co/namespace": b.Kibana.Namespace,
					},
				},
			)
		}
		if b.Kibana.Spec.HTTP.TLS.Enabled() {
			expected = append(expected,
				test.ExpectedSecret{
					Name: kbName + "-kb-http-ca-internal",
					Keys: []string{"tls.crt", "tls.key"},
					Labels: map[string]string{
						"kibana.k8s.elastic.co/name": kbName,
						"common.k8s.elastic.co/type": "kibana",
					},
				},
				test.ExpectedSecret{
					Name: kbName + "-kb-http-certs-internal",
					Keys: []string{"tls.crt", "tls.key", "ca.crt"},
					Labels: map[string]string{
						"kibana.k8s.elastic.co/name": kbName,
						"common.k8s.elastic.co/type": "kibana",
					},
				},
				test.ExpectedSecret{
					Name: kbName + "-kb-http-certs-public",
					Keys: []string{"ca.crt", "tls.crt"},
					Labels: map[string]string{
						"kibana.k8s.elastic.co/name": kbName,
						"common.k8s.elastic.co/type": "kibana",
					},
				},
			)
		}
		return expected
	})
}

func CheckStatus(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "Kibana status should be updated",
		Test: test.Eventually(func() error {
			var kb kbv1.Kibana
			if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.Kibana), &kb); err != nil {
				return err
			}
			// don't check the association status that may vary across tests
			kb.Status.AssociationStatus = ""
			expected := kbv1.KibanaStatus{
				DeploymentStatus: commonv1.DeploymentStatus{
					AvailableNodes: b.Kibana.Spec.Count,
					Version:        b.Kibana.Spec.Version,
					Health:         "green",
				},
				AssociationStatus: "",
			}
			if kb.Status != expected {
				return fmt.Errorf("expected status %+v but got %+v", expected, kb.Status)
			}
			return nil
		}),
	}
}

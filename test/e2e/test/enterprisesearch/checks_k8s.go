// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package enterprisesearch

import (
	"context"
	"fmt"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	entv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/enterprisesearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/checks"
)

func (b Builder) CheckK8sTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		checks.CheckDeployment(b, k, b.EnterpriseSearch.Name+"-ent"),
		checks.CheckPods(b, k),
		checks.CheckServices(b, k),
		checks.CheckServicesEndpoints(b, k),
		CheckSecrets(b, k),
		CheckStatus(b, k),
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

		if b.EnterpriseSearch.Spec.HTTP.TLS.Enabled() && !b.GlobalCA {
			expected = append(expected,
				test.ExpectedSecret{
					Name: entName + "-ent-http-ca-internal",
					Keys: []string{"tls.crt", "tls.key"},
					Labels: map[string]string{
						"enterprisesearch.k8s.elastic.co/name": entName,
						"common.k8s.elastic.co/type":           "enterprise-search",
					},
				},
			)
		}

		if b.EnterpriseSearch.Spec.HTTP.TLS.Enabled() {
			expected = append(expected,
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
			var ent entv1.EnterpriseSearch
			if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.EnterpriseSearch), &ent); err != nil {
				return err
			}

			// Selector is a string built from a map, it is validated with a dedicated function.
			// The expected value is hardcoded on purpose to ensure there is no regression in the way the set of labels
			// is created.
			if err := test.CheckSelector(
				ent.Status.Selector,
				map[string]string{
					"enterprisesearch.k8s.elastic.co/name": ent.Name,
					"common.k8s.elastic.co/type":           "enterprise-search",
				}); err != nil {
				return err
			}
			// don't check status fields that may vary across tests
			ent.Status.Selector = ""
			ent.Status.ObservedGeneration = 0

			expected := entv1.EnterpriseSearchStatus{
				DeploymentStatus: commonv1.DeploymentStatus{
					Count:          b.EnterpriseSearch.Spec.Count,
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

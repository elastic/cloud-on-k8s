// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package enterprisesearch

import (
	"context"
	"fmt"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	entv1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

func (b Builder) CheckK8sTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		test.CheckDeployment(b, k, b.EnterpriseSearch.Name+"-ent"),
		test.CheckPods(b, k),
		test.CheckServices(b, k),
		test.CheckServicesEndpoints(b, k),
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
			var ent entv1.EnterpriseSearch
			if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.EnterpriseSearch), &ent); err != nil {
				return err
			}
			expected := entv1.EnterpriseSearchStatus{
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

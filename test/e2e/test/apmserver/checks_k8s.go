// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package apmserver

import (
	"context"
	"fmt"

	apmv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/checks"
)

func (b Builder) CheckK8sTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		checks.CheckDeployment(b, k, b.ApmServer.Name+"-apm-server"),
		checks.CheckPods(b, k),
		checks.CheckServices(b, k),
		checks.CheckServicesEndpoints(b, k),
		CheckSecrets(b, k),
		CheckStatus(b, k),
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
			as.Status.ObservedGeneration = 0

			// Selector is a string built from a map, it is validated with a dedicated function.
			// The expected value is hardcoded on purpose to ensure there is no regression in the way the set of labels
			// is created.
			if err := test.CheckSelector(
				as.Status.Selector,
				map[string]string{
					"apm.k8s.elastic.co/name":    as.Name,
					"common.k8s.elastic.co/type": "apm-server",
				}); err != nil {
				return err
			}
			as.Status.Selector = ""

			expected := apmv1.ApmServerStatus{
				ExternalService:       b.ApmServer.Name + "-apm-http",
				SecretTokenSecretName: b.ApmServer.Name + "-apm-token",
				DeploymentStatus: commonv1.DeploymentStatus{
					Count:          b.ApmServer.Spec.Count,
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

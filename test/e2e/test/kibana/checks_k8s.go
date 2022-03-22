// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	"context"
	"fmt"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/checks"
)

func (b Builder) CheckK8sTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		checks.CheckDeployment(b, k, kbv1.Deployment(b.Kibana.Name)),
		checks.CheckPods(b, k),
		checks.CheckServices(b, k),
		checks.CheckServicesEndpoints(b, k),
		CheckSecrets(b, k),
		CheckStatus(b, k),
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
			)
			v, err := version.Parse(b.Kibana.Spec.Version)
			if err != nil {
				panic(err) // should not happen in an e2e test
			}
			if v.GTE(kbv1.KibanaServiceAccountMinVersion) {
				expected = append(expected,
					test.ExpectedSecret{
						Name: kbName + "-kibana-user",
						Keys: []string{"hash", "name", "serviceAccount", "token"},
						Labels: map[string]string{
							"eck.k8s.elastic.co/credentials":             "true",
							"elasticsearch.k8s.elastic.co/cluster-name":  b.Kibana.Spec.ElasticsearchRef.Name,
							"kibanaassociation.k8s.elastic.co/name":      kbName,
							"kibanaassociation.k8s.elastic.co/namespace": b.Kibana.Namespace,
						},
					},
				)
			} else {
				expected = append(expected,
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

			// Selector is a string built from a map, it is validated with a dedicated function.
			// The expected value is hardcoded on purpose to ensure there is no regression in the way the set of labels
			// is created.
			if err := test.CheckSelector(
				kb.Status.Selector,
				map[string]string{
					"kibana.k8s.elastic.co/name": kb.Name,
					"common.k8s.elastic.co/type": "kibana",
				}); err != nil {
				return err
			}
			kb.Status.Selector = ""

			// don't check the association statuses that may vary across tests
			expected := kbv1.KibanaStatus{
				DeploymentStatus: commonv1.DeploymentStatus{
					Count:          b.Kibana.Spec.Count,
					AvailableNodes: b.Kibana.Spec.Count,
					Version:        b.Kibana.Spec.Version,
					Health:         "green",
				},
			}
			if kb.Status.DeploymentStatus != expected.DeploymentStatus {
				return fmt.Errorf("expected status %+v but got %+v", expected, kb.Status)
			}
			return nil
		}),
	}
}

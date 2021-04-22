// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"context"
	"fmt"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

func (b Builder) CheckK8sTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		test.CheckDeployment(b, k, kibana.Deployment(b.Kibana.Name)),
		test.CheckPods(b, k),
		test.CheckServices(b, k),
		test.CheckServicesEndpoints(b, k),
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

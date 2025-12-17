// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package epr

import (
	"context"
	"fmt"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/apis/packageregistry/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
)

// CheckSecrets checks that expected secrets have been created.
func CheckSecrets(b Builder, k *test.K8sClient) test.Step {
	return test.CheckSecretsContent(k, b.EPR.Namespace, func() []test.ExpectedSecret {
		eprName := b.EPR.Name
		// hardcode all secret names and keys to catch any breaking change
		expected := []test.ExpectedSecret{
			{
				Name: eprName + "-epr-config",
				Keys: []string{"config.yml"},
				Labels: map[string]string{
					"packageregistry.k8s.elastic.co/name": eprName,
				},
			},
		}
		if b.EPR.Spec.HTTP.TLS.Enabled() {
			expected = append(expected,
				test.ExpectedSecret{
					Name: eprName + "-epr-http-ca-internal",
					Keys: []string{"tls.crt", "tls.key"},
					Labels: map[string]string{
						"packageregistry.k8s.elastic.co/name": eprName,
						"common.k8s.elastic.co/type":          "package-registry",
					},
				},
				test.ExpectedSecret{
					Name: eprName + "-epr-http-certs-internal",
					Keys: []string{"tls.crt", "tls.key", "ca.crt"},
					Labels: map[string]string{
						"packageregistry.k8s.elastic.co/name": eprName,
						"common.k8s.elastic.co/type":          "package-registry",
					},
				},
				test.ExpectedSecret{
					Name: eprName + "-epr-http-certs-public",
					Keys: []string{"ca.crt", "tls.crt"},
					Labels: map[string]string{
						"packageregistry.k8s.elastic.co/name": eprName,
						"common.k8s.elastic.co/type":          "package-registry",
					},
				},
			)
		}
		return expected
	})
}

func CheckStatus(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "Elastic Package Registry status should be updated",
		Test: test.Eventually(func() error {
			var epr v1alpha1.PackageRegistry
			if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.EPR), &epr); err != nil {
				return err
			}

			// Selector is a string built from a map, it is validated with a dedicated function.
			// The expected value is hardcoded on purpose to ensure there is no regression in the way the set of labels
			// is created.
			if err := test.CheckSelector(
				epr.Status.Selector,
				map[string]string{
					"packageregistry.k8s.elastic.co/name": epr.Name,
					"common.k8s.elastic.co/type":          "package-registry",
				}); err != nil {
				return err
			}
			epr.Status.Selector = ""
			epr.Status.ObservedGeneration = 0

			// don't check the association status that may vary across tests
			expected := v1alpha1.PackageRegistryStatus{
				DeploymentStatus: commonv1.DeploymentStatus{
					Count:          b.EPR.Spec.Count,
					AvailableNodes: b.EPR.Spec.Count,
					Version:        b.EPR.Spec.Version,
					Health:         "green",
				},
			}
			if epr.Status.DeploymentStatus != expected.DeploymentStatus {
				return fmt.Errorf("expected status %+v but got %+v", expected.DeploymentStatus, epr.Status.DeploymentStatus)
			}
			return nil
		}),
	}
}

func (b Builder) CheckStackTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Elastic Package Registry should respond to requests",
			Test: test.Eventually(func() error {
				client, err := NewEPRClient(b.EPR, k)
				if err != nil {
					return err
				}
				bytes, err := DoRequest(client, b.EPR, "GET", "/search")
				if err != nil {
					return err
				}
				// For EPR, just check that we get a valid response
				// EPR search endpoint returns package information
				if len(bytes) == 0 {
					return fmt.Errorf("expected non-empty response from EPR")
				}
				return nil
			}),
		},
	}
}

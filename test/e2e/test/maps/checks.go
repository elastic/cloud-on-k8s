// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package maps

import (
	"context"
	"encoding/json"
	"fmt"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/maps/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/set"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
)

// CheckSecrets checks that expected secrets have been created.
func CheckSecrets(b Builder, k *test.K8sClient) test.Step {
	return test.CheckSecretsContent(k, b.EMS.Namespace, func() []test.ExpectedSecret {
		emsName := b.EMS.Name
		// hardcode all secret names and keys to catch any breaking change
		expected := []test.ExpectedSecret{
			{
				Name: emsName + "-ems-config",
				Keys: []string{"elastic-maps-server.yml"},
				Labels: map[string]string{
					"eck.k8s.elastic.co/credentials": "true",
					"maps.k8s.elastic.co/name":       emsName,
				},
			},
		}
		if b.EMS.Spec.ElasticsearchRef.Name != "" {
			expected = append(expected,
				test.ExpectedSecret{
					Name: emsName + "-ems-es-ca",
					Keys: []string{"ca.crt", "tls.crt"},
					Labels: map[string]string{
						"elasticsearch.k8s.elastic.co/cluster-name": b.EMS.Spec.ElasticsearchRef.Name,
						"mapsassociation.k8s.elastic.co/name":       emsName,
						"mapsassociation.k8s.elastic.co/namespace":  b.EMS.Namespace,
					},
				},
				test.ExpectedSecret{
					Name: emsName + "-maps-user",
					Keys: []string{b.EMS.Namespace + "-" + emsName + "-maps-user"},
					Labels: map[string]string{
						"eck.k8s.elastic.co/credentials":            "true",
						"elasticsearch.k8s.elastic.co/cluster-name": b.EMS.Spec.ElasticsearchRef.Name,
						"mapsassociation.k8s.elastic.co/name":       emsName,
						"mapsassociation.k8s.elastic.co/namespace":  b.EMS.Namespace,
					},
				},
			)
		}
		if b.EMS.Spec.HTTP.TLS.Enabled() {
			expected = append(expected,
				test.ExpectedSecret{
					Name: emsName + "-ems-http-ca-internal",
					Keys: []string{"tls.crt", "tls.key"},
					Labels: map[string]string{
						"maps.k8s.elastic.co/name":   emsName,
						"common.k8s.elastic.co/type": "maps",
					},
				},
				test.ExpectedSecret{
					Name: emsName + "-ems-http-certs-internal",
					Keys: []string{"tls.crt", "tls.key", "ca.crt"},
					Labels: map[string]string{
						"maps.k8s.elastic.co/name":   emsName,
						"common.k8s.elastic.co/type": "maps",
					},
				},
				test.ExpectedSecret{
					Name: emsName + "-ems-http-certs-public",
					Keys: []string{"ca.crt", "tls.crt"},
					Labels: map[string]string{
						"maps.k8s.elastic.co/name":   emsName,
						"common.k8s.elastic.co/type": "maps",
					},
				},
			)
		}
		return expected
	})
}

func CheckStatus(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "Elastic Maps Server status should be updated",
		Test: test.Eventually(func() error {
			var ems v1alpha1.ElasticMapsServer
			if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.EMS), &ems); err != nil {
				return err
			}

			// Selector is a string built from a map, it is validated with a dedicated function.
			// The expected value is hardcoded on purpose to ensure there is no regression in the way the set of labels
			// is created.
			if err := test.CheckSelector(
				ems.Status.Selector,
				map[string]string{
					"maps.k8s.elastic.co/name":   ems.Name,
					"common.k8s.elastic.co/type": "maps",
				}); err != nil {
				return err
			}
			ems.Status.Selector = ""
			ems.Status.ObservedGeneration = 0

			// don't check the association status that may vary across tests
			ems.Status.AssociationStatus = ""
			expected := v1alpha1.MapsStatus{
				DeploymentStatus: commonv1.DeploymentStatus{
					Count:          b.EMS.Spec.Count,
					AvailableNodes: b.EMS.Spec.Count,
					Version:        b.EMS.Spec.Version,
					Health:         "green",
				},
				AssociationStatus: "",
			}
			if ems.Status != expected {
				return fmt.Errorf("expected status %+v but got %+v", expected, ems.Status)
			}
			return nil
		}),
	}
}

type emsStatus struct {
	Version string `json:"version"`
	Overall struct {
		State string `json:"state"`
	} `json:"overall"`
}

func (b Builder) CheckStackTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Elastic Maps Server should respond to requests",
			Test: test.Eventually(func() error {
				client, err := NewMapsClient(b.EMS, k)
				if err != nil {
					return err
				}
				bytes, err := DoRequest(client, b.EMS, "GET", "/status")
				if err != nil {
					return err
				}
				var status emsStatus
				if err := json.Unmarshal(bytes, &status); err != nil {
					return err
				}
				// 7.11-7.12 reports `degraded` status when the full basemap is not installed
				expectedStatus := set.Make("available", "degraded")
				if !expectedStatus.Has(status.Overall.State) {
					return fmt.Errorf("expected one of %v but got %s", expectedStatus, status.Overall.State)
				}
				return nil
			}),
		},
	}
}

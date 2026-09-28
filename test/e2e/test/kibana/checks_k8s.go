// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	"context"
	"fmt"

	"github.com/google/go-cmp/cmp"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/checks"
)

// deploymentCountSubject wraps Builder and overrides Count() for a single deployment check.
// checks.CheckDeployment compares Count() against the deployment's replica count; when background
// task isolation is active, Count() returns the total across pools, which would be wrong for each
// individual deployment. This wrapper pins Count() to the per-deployment value.
type deploymentCountSubject struct {
	Builder
	count int32
}

func (d deploymentCountSubject) Count() int32 { return d.count }

func (b Builder) CheckK8sTestSteps(k *test.K8sClient) test.StepList {
	steps := test.StepList{
		checks.CheckDeployment(
			deploymentCountSubject{Builder: b, count: b.Kibana.Spec.Count},
			k,
			kbv1.Deployment(b.Kibana.Name),
		),
	}

	// check background tasks deployment if it is enabled.
	if b.Kibana.BackgroundTasksEnabled() {
		bgDeploymentName := kbv1.BackgroundTasksDeployment(b.Kibana.Name)
		if b.Kibana.Spec.BackgroundTasks.Count != nil {
			// Count is managed by ECK: assert exact replica count.
			bgCount := *b.Kibana.Spec.BackgroundTasks.Count
			steps = append(steps, checks.CheckDeployment(
				deploymentCountSubject{Builder: b, count: bgCount},
				k,
				bgDeploymentName,
			))
		} else {
			// Count is nil: an HPA manages replicas. Only assert the deployment exists.
			steps = append(steps, test.Step{
				Name: "background tasks Deployment should exist (HPA-managed)",
				Test: test.Eventually(func() error {
					var dep appsv1.Deployment
					return k.Client.Get(context.Background(), types.NamespacedName{
						Namespace: b.Kibana.Namespace,
						Name:      bgDeploymentName,
					}, &dep)
				}),
			})
		}
	}

	return steps.WithSteps(test.StepList{
		checks.CheckPods(b, k),
		checks.CheckServices(b, k),
		checks.CheckServicesEndpoints(b, k),
		CheckSecrets(b, k),
		CheckStatus(b, k),
		test.CheckFieldsNotOwnedByOperator(&b.Kibana, k, nil),
	})
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
					"kibana.k8s.elastic.co/name": kbName,
				},
			},
		}
		if b.Kibana.Spec.ElasticsearchRef.Name != "" {
			expected = append(
				expected,
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
				expected = append(
					expected,
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
				expected = append(
					expected,
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
		if b.Kibana.Spec.PackageRegistryRef.Name != "" {
			expected = append(
				expected,
				test.ExpectedSecret{
					Name: kbName + "-kb-epr-ca",
					Keys: []string{"ca.crt", "tls.crt"},
					Labels: map[string]string{
						"packageregistry.k8s.elastic.co/name":        b.Kibana.Spec.PackageRegistryRef.Name,
						"kibanaassociation.k8s.elastic.co/name":      kbName,
						"kibanaassociation.k8s.elastic.co/namespace": b.Kibana.Namespace,
					},
				},
			)
		}
		if b.Kibana.Spec.HTTP.TLS.Enabled() {
			expected = append(
				expected,
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
		if b.Kibana.Spec.HTTP.TLS.Enabled() && !b.GlobalCA {
			expected = append(
				expected,
				test.ExpectedSecret{
					Name: kbName + "-kb-http-ca-internal",
					Keys: []string{"tls.crt", "tls.key"},
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
				},
			); err != nil {
				return err
			}
			kb.Status.Selector = ""

			// don't check the association statuses that may vary across tests
			// Count is the UI pool's replica count (drives the scale sub-resource).
			// AvailableNodes is the controller-aggregated count across all active pools.
			expected := kbv1.KibanaStatus{
				DeploymentStatus: commonv1.DeploymentStatus{
					Count:          b.Kibana.Spec.Count,
					AvailableNodes: b.Count(),
					Version:        b.Kibana.Spec.Version,
					Health:         "green",
					Conditions:     kb.Status.Conditions, // Ignore Conditions whose LastTransitionTime is unpredictable
				},
			}

			if !cmp.Equal(kb.Status.DeploymentStatus, expected.DeploymentStatus) {
				return fmt.Errorf("expected status %+v but got %+v", expected.DeploymentStatus, kb.Status.DeploymentStatus)
			}
			return nil
		}),
	}
}

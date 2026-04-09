// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build es || e2e

package es

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/stackmon/validations"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/checks"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
)

// nodePort is a nodePort usable to expose and reach any service through a public IP.
// The port has a corresponding firewall rule to be allowed from any sources:
// - gcloud compute firewall-rules create eck-e2e-tests-node-port --allow tcp:32767
const nodePort = int32(32767)

// TestESStackMonitoring tests that when an Elasticsearch cluster is configured with monitoring, its log and metrics are
// correctly delivered to the referenced monitoring Elasticsearch clusters.
func TestESStackMonitoring(t *testing.T) {
	// only execute this test on supported version
	err := validations.IsSupportedVersion(test.Ctx().ElasticStackVersion, validations.MinStackVersion)
	if err != nil {
		t.SkipNow()
	}

	// create 1 monitored and 2 monitoring clusters to collect separately metrics and logs
	metrics := elasticsearch.NewBuilder("test-es-mon-metrics").
		WithESMasterDataNodes(2, elasticsearch.DefaultResources)
	logs := elasticsearch.NewBuilder("test-es-mon-logs").
		WithESMasterDataNodes(2, elasticsearch.DefaultResources)
	monitored := elasticsearch.NewBuilder("test-es-mon-a").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithMonitoring(metrics.Ref(), logs.Ref())

	// checks that the sidecar beats have sent data in the monitoring clusters
	steps := func(k *test.K8sClient) test.StepList {
		return checks.MonitoredSteps(&monitored, k)
	}

	test.Sequence(nil, steps, metrics, logs, monitored).RunSequential(t)
}

// TestExternalESStackMonitoring tests the Stack Monitoring feature with an association to a simulated external Elasticsearch using a secret ref.
// The external Elasticsearch is managed by ECK but not directly associated to the monitored cluster, instead a monitoring user is created and a
// secret with the corresponding info is created.
func TestExternalESStackMonitoring(t *testing.T) {
	// only execute this test on GKE where k8s nodes have public IPs and nodePort 32767 is open
	if !test.IsGKE(test.Ctx().KubernetesVersion) {
		t.SkipNow()
	}
	// only execute this test on supported version
	err := validations.IsSupportedVersion(test.Ctx().ElasticStackVersion, validations.MinStackVersion)
	if err != nil {
		t.SkipNow()
	}

	// create 1 monitored and 2 monitoring clusters to collect separately metrics and logs
	monitoring := elasticsearch.NewBuilder("test-es-mon").
		WithESMasterDataNodes(2, elasticsearch.DefaultResources)

	// do not associate the two clusters right now
	monitored := elasticsearch.NewBuilder("test-es-mon-a").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)
	nodeExternalIP := ""

	extRefSecretName := "test-es-mon-ext-ref"
	extRefSecretNamespace := test.Ctx().ManagedNamespace(0)
	extRefUsername := "mon-user"
	extRefPassword := common.FixedLengthRandomPasswordBytes()

	steps := func(k *test.K8sClient) test.StepList {
		s := test.StepList{
			test.Step{
				Name: "Get external k8s node IP",
				Test: test.Eventually(func() error {
					var err error
					nodeExternalIP, err = k.GetFirstNodeExternalIP()
					return err
				}),
			},
			test.Step{
				Name: "Configure monitoring ES cluster with NodePort service and k8s node IP in the SAN of the self-signed certificate",
				Test: test.Eventually(func() error {
					var es esv1.Elasticsearch
					if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&monitoring.Elasticsearch), &es); err != nil {
						return err
					}

					es.Spec.HTTP = commonv1.HTTPConfigWithClientOptions{
						Service: commonv1.ServiceTemplate{
							Spec: corev1.ServiceSpec{
								Type: corev1.ServiceTypeNodePort,
								Ports: []corev1.ServicePort{
									{Port: 9200, NodePort: nodePort},
								},
							},
						},
						TLS: commonv1.TLSWithClientOptions{
							TLSOptions: commonv1.TLSOptions{
								SelfSignedCertificate: &commonv1.SelfSignedCertificate{
									SubjectAlternativeNames: []commonv1.SubjectAlternativeName{
										{IP: nodeExternalIP},
									},
								},
							},
						},
					}

					return k.Client.Update(context.Background(), &es)
				}),
			},
			test.Step{
				Name: "Create a monitoring user",
				Test: test.Eventually(func() error {
					esClient, err := elasticsearch.NewElasticsearchClient(monitoring.Elasticsearch, k)
					if err != nil {
						return err
					}

					body := bytes.NewBufferString(`{
						"username":"` + extRefUsername + `","password":"` + string(extRefPassword) + `",
						"roles":["monitoring_user","kibana_admin","remote_monitoring_agent","remote_monitoring_collector"]}`)
					req, err := http.NewRequest(http.MethodPost, "/_security/user/"+extRefUsername, body)
					if err != nil {
						return err
					}

					_, err = esClient.Request(context.Background(), req)
					return err
				}),
			},
			test.Step{
				Name: "Create a secret to reference the monitoring cluster",
				Test: test.Eventually(func() error {
					var monitoringHTTPPublicCertsSecret corev1.Secret
					key := types.NamespacedName{
						Namespace: monitoring.Elasticsearch.Namespace,
						Name:      certificates.PublicCertsSecretName(esv1.ESNamer, monitoring.Elasticsearch.Name),
					}

					if err := k.Client.Get(context.Background(), key, &monitoringHTTPPublicCertsSecret); err != nil {
						return err
					}
					if err != nil {
						return err
					}

					refSecret := corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: extRefSecretNamespace,
							Name:      extRefSecretName,
						},
						Data: map[string][]byte{
							"url":      fmt.Appendf(nil, "https://%s:%d", nodeExternalIP, nodePort),
							"username": []byte(extRefUsername),
							"password": extRefPassword,
							"ca.crt":   monitoringHTTPPublicCertsSecret.Data["ca.crt"],
						},
					}

					return k.CreateOrUpdate(&refSecret)
				}),
			},
			test.Step{
				Name: "Update monitored es cluster with the secret reference",
				Test: test.Eventually(func() error {
					var es esv1.Elasticsearch
					if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&monitored.Elasticsearch), &es); err != nil {
						return err
					}

					monitoringEsRef := []commonv1.ObjectSelector{{SecretName: extRefSecretName}}
					es.Spec.Monitoring = commonv1.Monitoring{
						Metrics: commonv1.MetricsMonitoring{ElasticsearchRefs: monitoringEsRef},
						Logs:    commonv1.LogsMonitoring{ElasticsearchRefs: monitoringEsRef},
					}

					return k.Client.Update(context.Background(), &es)
				}),
			},
		}

		c := checks.MonitoredSteps(&monitored, k)
		return append(s, c...)
	}

	test.Sequence(nil, steps, monitoring, monitored).RunSequential(t)
}

// checkClientAuthAnnotationStep returns a test step that verifies whether the given Elasticsearch resource
// has (or does not have) the client authentication required annotation.
func checkClientAuthAnnotationStep(k *test.K8sClient, namespace, esName string, expected bool) test.Step {
	return test.Step{
		Name: fmt.Sprintf("Verify ES %s/%s has client authentication annotation = %v", namespace, esName, expected),
		Test: test.Eventually(func() error {
			var es esv1.Elasticsearch
			if err := k.Client.Get(context.Background(), types.NamespacedName{
				Namespace: namespace,
				Name:      esName,
			}, &es); err != nil {
				return err
			}
			if annotation.HasClientAuthenticationRequired(&es) != expected {
				return fmt.Errorf("expected client authentication required annotation to be %v for ES %s/%s", expected, namespace, esName)
			}
			return nil
		}),
	}
}

// TestESStackClientAuthTransitionMonitored tests that when a monitored Elasticsearch cluster transitions
// from having client authentication enabled to disabled, the monitoring continues to work correctly.
func TestESStackClientAuthTransitionMonitored(t *testing.T) {
	if test.Ctx().TestLicense == "" {
		t.Skip("Skipping client authentication test: no enterprise test license configured")
	}
	// only execute this test on supported version
	err := validations.IsSupportedVersion(test.Ctx().ElasticStackVersion, validations.MinStackVersion)
	if err != nil {
		t.SkipNow()
	}

	// create monitoring cluster without client authentication
	monitoring := elasticsearch.NewBuilder("test-es-mon-trans").
		WithESMasterDataNodes(2, elasticsearch.DefaultResources).
		WithClientAuthenticationRequired()

	// create monitored cluster with client authentication required initially
	monitored := elasticsearch.NewBuilder("test-es-mon-trans-a").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithClientAuthenticationRequired().
		WithMonitoring(monitoring.Ref(), monitoring.Ref())

	monitoringWithLicense := test.LicenseTestBuilder(monitoring)
	monitoredWithLicense := test.LicenseTestBuilder(monitored)
	monitoredWithLicense.PostCheckSteps = func(k *test.K8sClient) test.StepList {
		return test.StepList{
			checkClientAuthAnnotationStep(k, monitored.Elasticsearch.Namespace, monitored.Elasticsearch.Name, true),
		}.WithSteps(checks.MonitoredSteps(&monitored, k))
	}

	// Disable client authentication on monitored cluster
	monitoredMutated := monitored.DeepCopy().WithMutatedFrom(&monitored)
	monitoredMutated.Elasticsearch.Spec.HTTP.TLS.Client.Authentication = false
	monitoredMutatedWrapped := test.WrappedBuilder{
		BuildingThis: monitoredMutated,
		PostMutationSteps: func(k *test.K8sClient) test.StepList {
			return test.StepList{
				checkClientAuthAnnotationStep(k, monitored.Elasticsearch.Namespace, monitored.Elasticsearch.Name, false),
			}.WithSteps(checks.MonitoredSteps(monitoredMutated, k))
		},
	}

	test.RunMutations(t, []test.Builder{monitoringWithLicense, monitoredWithLicense}, []test.Builder{monitoredMutatedWrapped})
}

// TestESStackClientAuthTransitionMonitoring tests that when the monitoring Elasticsearch cluster
// transitions from having client authentication enabled to disabled, the monitoring continues to work correctly.
func TestESStackClientAuthTransitionMonitoring(t *testing.T) {
	if test.Ctx().TestLicense == "" {
		t.Skip("Skipping client authentication test: no enterprise test license configured")
	}
	// only execute this test on supported version
	err := validations.IsSupportedVersion(test.Ctx().ElasticStackVersion, validations.MinStackVersion)
	if err != nil {
		t.SkipNow()
	}

	// create monitoring cluster with client authentication required initially
	monitoring := elasticsearch.NewBuilder("test-es-mmon-trans").
		WithESMasterDataNodes(2, elasticsearch.DefaultResources).
		WithClientAuthenticationRequired()

	// create monitored cluster without client authentication
	monitored := elasticsearch.NewBuilder("test-es-mmon-trans-a").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithClientAuthenticationRequired().
		WithMonitoring(monitoring.Ref(), monitoring.Ref())

	monitoringWithLicense := test.LicenseTestBuilder(monitoring)
	monitoringWithLicense.PostCheckSteps = func(k *test.K8sClient) test.StepList {
		return test.StepList{
			checkClientAuthAnnotationStep(k, monitoring.Elasticsearch.Namespace, monitoring.Elasticsearch.Name, true),
		}
	}
	monitoredWithLicense := test.LicenseTestBuilder(monitored)
	monitoredWithLicense.PostCheckSteps = func(k *test.K8sClient) test.StepList {
		return checks.MonitoredSteps(&monitored, k)
	}

	// Disable client authentication on monitoring cluster
	monitoringMutated := monitoring.DeepCopy().WithMutatedFrom(&monitoring)
	monitoringMutated.Elasticsearch.Spec.HTTP.TLS.Client.Authentication = false
	monitoringMutatedWrapped := test.WrappedBuilder{
		BuildingThis: monitoringMutated,
		PostMutationSteps: func(k *test.K8sClient) test.StepList {
			return test.StepList{
				checkClientAuthAnnotationStep(k, monitoring.Elasticsearch.Namespace, monitoring.Elasticsearch.Name, false),
			}.WithSteps(checks.MonitoredSteps(&monitored, k))
		},
	}

	test.RunMutations(t, []test.Builder{monitoringWithLicense, monitoredWithLicense}, []test.Builder{monitoringMutatedWrapped})
}

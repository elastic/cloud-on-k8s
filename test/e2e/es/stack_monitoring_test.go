// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// +build es e2e

package es

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/stackmon/validations"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/checks"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
)

// TestESStackMonitoring tests that when an Elasticsearch cluster is configured with monitoring, its log and metrics are
// correctly delivered to the referenced monitoring Elasticsearch clusters.
func TestESStackMonitoring(t *testing.T) {
	// only execute this test on supported version
	err := validations.IsSupportedVersion(test.Ctx().ElasticStackVersion)
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
	// only execute this test on supported version
	err := validations.IsSupportedVersion(test.Ctx().ElasticStackVersion)
	if err != nil {
		t.SkipNow()
	}

	// create 1 monitored and 2 monitoring clusters to collect separately metrics and logs
	monitoring := elasticsearch.NewBuilder("test-es-mon").
		WithESMasterDataNodes(2, elasticsearch.DefaultResources)

	// do not associate the two clusters right now
	monitored := elasticsearch.NewBuilder("test-es-mon-a").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	steps := func(k *test.K8sClient) test.StepList {
		secretName := "test-es-mon-ext-ref"
		secretNamespace := test.Ctx().ManagedNamespace(0)
		var esURL *string
		username := "mon-user"
		password := "mon-pwd"

		s := test.StepList{
			test.Step{
				Name: "Create a monitoring user",
				Test: test.Eventually(func() error {
					esClient, err := elasticsearch.NewElasticsearchClient(monitoring.Elasticsearch, k)
					if err != nil {
						return err
					}

					url := esClient.URL()
					esURL = &url

					body := bytes.NewBufferString(`{"username":"`+username+`","password":"`+password+`","roles":["monitoring_user","kibana_admin","remote_monitoring_agent","remote_monitoring_collector"]}`)
					req, err := http.NewRequest(http.MethodPost, "/_security/user/"+username, body)
					if err != nil {
						return err
					}


					_, err = esClient.Request(context.Background(), req)
					if err != nil {
						return err
					}

					return nil
				}),
			},
			test.Step{
				Name: "Create a secret to reference the monitoring cluster",
				Test: test.Eventually(func() error {

					var secret corev1.Secret
					key := types.NamespacedName{
						Namespace: monitoring.Elasticsearch.Namespace,
						Name:      certificates.PublicCertsSecretName(esv1.ESNamer, monitoring.Elasticsearch.Name),
					}
					if err := k.Client.Get(context.Background(), key, &secret); err != nil {
						return err
					}
					if err != nil {
						return err
					}

					refSecret := corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: secretNamespace,
							Name:      secretName,
						},
						Data: map[string][]byte{
							"url": []byte(*esURL),
							"username": []byte(username),
							"password": []byte(password),
							"ca.crt": secret.Data["ca.crt"],
						},
					}
					err = k.CreateOrUpdate(&refSecret)
					if err != nil {
						t.Log("create secret err", err)
						return err
					}

					return nil
				}),
			},
			test.Step{
				Name: "Update monitored es cluster with the secret reference",
				Test: test.Eventually(func() error {
					ref := commonv1.ObjectSelector{
						Namespace:  secretNamespace,
						SecretName: secretName,
					}

					var es esv1.Elasticsearch
					if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&monitored.Elasticsearch), &es); err != nil {
						return err
					}

					es.Spec.Monitoring = esv1.Monitoring{
						Metrics: esv1.MetricsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{ref}},
						Logs: esv1.LogsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{ref}},
					}
					err := k.Client.Update(context.Background(), &es)
					if err != nil {
						t.Log("update es err", err)
						return err
					}
					t.Log("update es", es.Spec.Monitoring)
					return nil
				}),
			},
		}
		c := checks.MonitoredSteps(&monitored, k)
		return append(s, c...)
	}

	test.Sequence(nil, steps, monitoring, monitored).RunSequential(t)
}

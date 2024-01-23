// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build logstash || e2e

package logstash

import (
	"context"
	"errors"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/configs"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/checks"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/logstash"
)

// TestLogstashStackMonitoring tests that when Logstash is configured with monitoring, its log and metrics are
// correctly delivered to the referenced monitoring Elasticsearch clusters.
func TestLogstashStackMonitoring(t *testing.T) {
	// create 1 monitored and 2 monitoring clusters to collect separately metrics and logs
	metrics := elasticsearch.NewBuilder("test-ls-mon-metrics").
		WithESMasterDataNodes(2, elasticsearch.DefaultResources)
	logs := elasticsearch.NewBuilder("test-ls-mon-logs").
		WithESMasterDataNodes(2, elasticsearch.DefaultResources)
	monitored := logstash.NewBuilder("test-ls-mon-a").
		WithNodeCount(1).
		WithMetricsMonitoring(metrics.Ref()).
		WithLogsMonitoring(logs.Ref()).
		WithConfig(map[string]interface{}{
			"api.auth.type":           "basic",
			"api.auth.basic.username": "logstash",
			"api.auth.basic.password": "changeit",
		})

	// checks that the sidecar beats have sent data in the monitoring clusters
	steps := func(k *test.K8sClient) test.StepList {
		return checks.MonitoredSteps(&monitored, k)
	}

	test.Sequence(nil, steps, metrics, logs, monitored).RunSequential(t)
}

// TestLogstashResolvingDollarVariableInStackMonitoring tests that the dollar sign variable is resolved in correct sequence, and
// Logstash API server setup correctly, and Beats is able to collect metrics using username password.
func TestLogstashResolvingDollarVariableInStackMonitoring(t *testing.T) {
	secureSettingSecretName := "test-ls-mon-secure-settings"
	username := "batman"
	password := "i_am_rich$"
	keystorePassword := "whatever"

	secureSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secureSettingSecretName,
			Namespace: test.Ctx().ManagedNamespace(0),
		},
		StringData: map[string]string{
			"API_USERNAME":          username,
			"SSL_KEYSTORE_PASSWORD": keystorePassword,
		},
	}

	before := test.StepsFunc(func(k *test.K8sClient) test.StepList {
		return test.StepList{}.WithStep(test.Step{
			Name: "Create secret for keystore",
			Test: test.Eventually(func() error {
				return k.CreateOrUpdateSecrets(secureSecret)
			}),
		})
	})

	// create 1 monitored and 2 monitoring clusters to collect separately metrics and logs
	// ${VAR} resolve from keystore and env. Keystore takes the precedence.
	metrics := elasticsearch.NewBuilder("test-ls-mon-metrics").
		WithESMasterDataNodes(2, elasticsearch.DefaultResources)
	logs := elasticsearch.NewBuilder("test-ls-mon-logs").
		WithESMasterDataNodes(2, elasticsearch.DefaultResources)
	monitored := logstash.NewBuilder("test-ls-mon-dollar").
		WithNodeCount(1).
		WithMetricsMonitoring(metrics.Ref()).
		WithLogsMonitoring(logs.Ref()).
		WithSecureSettings(commonv1.SecretSource{SecretName: secureSettingSecretName}).
		WithPodTemplate(corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "logstash",
						Env: []corev1.EnvVar{
							{
								Name:  "API_PASSWORD",
								Value: password,
							},
							{
								Name:  "API_USERNAME",
								Value: "ENV_HAS_LOWER_ORDER",
							},
						},
					},
				},
			},
		}).
		WithConfig(map[string]interface{}{
			"api.ssl.keystore.password": "${SSL_KEYSTORE_PASSWORD}",
			"api.auth.type":             "basic",
			"api.auth.basic.username":   "${API_USERNAME}",
			"api.auth.basic.password":   "${API_PASSWORD}",
		}).
		WithExpectedAPIServer(configs.APIServer{Username: username, Password: password})

	// checks that the sidecar beats have sent data in the monitoring clusters
	// check config secret has correct API_KEYSTORE_PASS value
	steps := func(k *test.K8sClient) test.StepList {
		return checks.MonitoredSteps(&monitored, k).
			WithStep(test.Step{
				Name: "Keystore password stored in config secret",
				Test: test.Eventually(func() error {
					nsn := types.NamespacedName{Namespace: monitored.Namespace(), Name: logstashv1alpha1.ConfigSecretName(monitored.Name())}
					var s corev1.Secret
					if err := k.Client.Get(context.Background(), nsn, &s); err != nil {
						return err
					}
					secretVal := string(s.Data["API_KEYSTORE_PASS"])
					if secretVal != keystorePassword {
						return errors.New(fmt.Sprintf("API_KEYSTORE_PASS in config secret does not match SSL_KEYSTORE_PASSWORD in keystore secret. Got: %s. Expected: %s.", secretVal, keystorePassword))
					}
					return nil
				}),
			}).
			WithStep(test.Step{
				Name: "Delete secret for keystore",
				Test: test.Eventually(func() error {
					err := k.Client.Delete(context.Background(), &secureSecret)
					if err != nil && !apierrors.IsNotFound(err) {
						return err
					}
					return nil
				}),
			})
	}

	test.Sequence(before, steps, metrics, logs, monitored).RunSequential(t)
}

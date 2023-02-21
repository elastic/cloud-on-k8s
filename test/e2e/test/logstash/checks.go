// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"context"
	"encoding/json"
	"fmt"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
)

type logstashStatus struct {
	Version string `json:"version"`
	Status  string `json:"status"`
}

// CheckSecrets checks that expected secrets have been created.
func CheckSecrets(b Builder, k *test.K8sClient) test.Step {
	return test.CheckSecretsContent(k, b.Logstash.Namespace, func() []test.ExpectedSecret {
		logstashName := b.Logstash.Name
		// hardcode all secret names and keys to catch any breaking change
		expected := []test.ExpectedSecret{
			{
				Name: logstashName + "-ls-config",
				Keys: []string{"logstash.yml"},
				Labels: map[string]string{
					"eck.k8s.elastic.co/credentials": "true",
					"logstash.k8s.elastic.co/name":   logstashName,
				},
			},
		}
		return expected
	})
}

func CheckStatus(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "Logstash status should have the correct status",
		Test: test.Eventually(func() error {
			var logstash logstashv1alpha1.Logstash
			if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.Logstash), &logstash); err != nil {
				return err
			}

			logstash.Status.ObservedGeneration = 0

			expected := logstashv1alpha1.LogstashStatus{
				ExpectedNodes:  b.Logstash.Spec.Count,
				AvailableNodes: b.Logstash.Spec.Count,
				Version:        b.Logstash.Spec.Version,
			}

			if (logstash.Status.ExpectedNodes != expected.ExpectedNodes) ||
				(logstash.Status.AvailableNodes != expected.AvailableNodes) ||
				(logstash.Status.Version != expected.Version) {
				return fmt.Errorf("expected status %+v but got %+v", expected, logstash.Status)
			}
			return nil
		}),
	}
}

func (b Builder) CheckStackTestSteps(k *test.K8sClient) test.StepList {
	println(test.Ctx().TestTimeout)
	return test.StepList{
		{
			Name: "Logstash should respond to requests",
			Test: test.Eventually(func() error {
				client, err := NewLogstashClient(b.Logstash, k)
				if err != nil {
					return err
				}
				bytes, err := DoRequest(client, b.Logstash, "GET", "/")
				if err != nil {
					return err
				}
				var status logstashStatus
				if err := json.Unmarshal(bytes, &status); err != nil {
					return err
				}

				if status.Status != "green" {
					return fmt.Errorf("expected green but got %s", status.Status)
				}
				return nil
			}),
		},
	}
}

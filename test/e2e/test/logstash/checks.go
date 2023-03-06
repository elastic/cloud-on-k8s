// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"

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
			if logstash.Status != expected {
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

func CheckServices(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "Logstash services should be created",
		Test: test.Eventually(func() error {
			serviceNames := map[string]struct{}{}
			serviceNames[logstashv1alpha1.APIServiceName(b.Logstash.Name)] = struct{}{}
			for _, r := range b.Logstash.Spec.Services {
				serviceNames[logstashv1alpha1.UserServiceName(b.Logstash.Name, r.Name)] = struct{}{}
			}
			for serviceName := range serviceNames {
				svc, err := k.GetService(b.Logstash.Namespace, serviceName)
				if err != nil {
					return err
				}
				if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
					if len(svc.Status.LoadBalancer.Ingress) == 0 {
						return fmt.Errorf("load balancer for %s not ready yet", svc.Name)
					}
				}
			}
			return nil
		}),
	}
}

// CheckServicesEndpoints checks that services have the expected number of endpoints
func CheckServicesEndpoints(b Builder, k *test.K8sClient) test.Step {
	return test.Step{
		Name: "Logstash services should have endpoints",
		Test: test.Eventually(func() error {
			servicePorts := make(map[string]int32)
			servicePorts[logstashv1alpha1.APIServiceName(b.Logstash.Name)] = b.Logstash.Spec.Count
			for _, r := range b.Logstash.Spec.Services {
				portsPerService := int32(len(r.Service.Spec.Ports))
				servicePorts[logstashv1alpha1.UserServiceName(b.Logstash.Name, r.Name)] = b.Logstash.Spec.Count * portsPerService
			}

			for endpointName, addrPortCount := range servicePorts {
				if addrPortCount == 0 {
					continue
				}
				endpoints, err := k.GetEndpoints(b.Logstash.Namespace, endpointName)
				if err != nil {
					return err
				}
				if len(endpoints.Subsets) == 0 {
					return fmt.Errorf("no subset for endpoint %s", endpointName)
				}
				if int32(len(endpoints.Subsets[0].Addresses)*len(endpoints.Subsets[0].Ports)) != addrPortCount {
					return fmt.Errorf("%d addresses and %d ports found for endpoint %s, expected %d", len(endpoints.Subsets[0].Addresses),
						len(endpoints.Subsets[0].Ports), endpointName, addrPortCount)
				}
			}
			return nil
		}),
	}
}

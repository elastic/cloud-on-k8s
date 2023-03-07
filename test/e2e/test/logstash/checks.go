// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/settings"

	corev1 "k8s.io/api/core/v1"

	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
)

type Request struct {
	Name string
	Path string
}

type Want struct {
	Status string
	// Key is field path of ucfg.Config. Value is the expected string
	// example, pipelines.demo.batch_size : 2
	Match map[string]string
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
			{
				Name: logstashName + "-ls-pipeline",
				Keys: []string{"pipelines.yml"},
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
	return test.StepList{
		b.CheckMetricsRequest(k,
			Request{
				Name: "metrics",
				Path: "/",
			},
			Want{
				Status: "green",
			}),
		b.CheckMetricsRequest(k,
			Request{
				Name: "default pipeline",
				Path: "/_node/pipelines/demo",
			},
			Want{
				Status: "green",
				Match:  map[string]string{"pipelines.demo.workers": "2"},
			}),
	}
}

func (b Builder) CheckMetricsRequest(k *test.K8sClient, req Request, want Want) test.Step {
	return test.Step{
		Name: fmt.Sprintf("Logstash should respond to %s requests", req.Name),
		Test: func(t *testing.T) {
			t.Helper()

			// send request and parse to map obj
			client, err := NewLogstashClient(b.Logstash, k)
			require.NoError(t, err)

			bytes, err := DoRequest(client, b.Logstash, "GET", req.Path)
			require.NoError(t, err)

			var response map[string]interface{}
			err = json.Unmarshal(bytes, &response)
			require.NoError(t, err)

			// parse response to ucfg.Config for traverse
			res, err := settings.NewCanonicalConfigFrom(response)
			require.NoError(t, err)

			// check status
			status, err := res.String("status")
			require.NoError(t, err)
			if status != want.Status {
				require.NoError(t, fmt.Errorf("expected %s but got %s", want.Status, status))
			}

			// check expected string
			for k, v := range want.Match {
				str, err := res.String(k)
				require.NoError(t, err)
				if str != v {
					require.NoError(t, fmt.Errorf("expected %s to be %s but got %s", k, v, str))
				}
			}
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

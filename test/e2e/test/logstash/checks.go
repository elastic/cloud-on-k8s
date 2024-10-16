// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	v1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
)

type Request struct {
	Name     string
	Path     string
	Username string
	Password string
}

type Want struct {
	// Key is field path of ucfg.Config. Value is the expected string
	// example, pipelines.demo.batch_size : 2
	Match     map[string]string
	MatchFunc map[string]func(string) bool
}

// CheckSecrets checks that expected secrets have been created.
func CheckSecrets(b Builder, k *test.K8sClient) test.Step {
	return test.CheckSecretsContent(k, b.Logstash.Namespace, func() []test.ExpectedSecret {
		logstashName := b.Logstash.Name
		// hardcode all secret names and keys to catch any breaking change
		expected := []test.ExpectedSecret{
			{
				Name: logstashName + "-ls-config",
				Keys: []string{"logstash.yml", "API_KEYSTORE_PASS"},
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

		// check ES association user/ secret
		nn := k8s.ExtractNamespacedName(&b.Logstash)
		lsName := nn.Name
		lsNamespace := nn.Namespace

		for _, ref := range b.Logstash.Spec.ElasticsearchRefs {
			esNamespace := ref.WithDefaultNamespace(lsNamespace).Namespace
			expected = append(expected,
				test.ExpectedSecret{
					Name: fmt.Sprintf("%s-logstash-es-%s-%s-ca", lsName, esNamespace, ref.Name),
					Keys: []string{"ca.crt", "tls.crt"},
					Labels: map[string]string{
						"elasticsearch.k8s.elastic.co/cluster-name":      ref.Name,
						"elasticsearch.k8s.elastic.co/cluster-namespace": esNamespace,
						"logstashassociation.k8s.elastic.co/name":        lsName,
						"logstashassociation.k8s.elastic.co/namespace":   lsNamespace,
					},
				},
			)
			expected = append(expected,
				test.ExpectedSecret{
					Name: fmt.Sprintf("%s-%s-%s-%s-logstash-user", lsNamespace, lsName, esNamespace, ref.Name),
					Keys: []string{"name", "passwordHash", "userRoles"},
					Labels: map[string]string{
						"elasticsearch.k8s.elastic.co/cluster-name":      ref.Name,
						"elasticsearch.k8s.elastic.co/cluster-namespace": esNamespace,
						"logstashassociation.k8s.elastic.co/name":        lsName,
						"logstashassociation.k8s.elastic.co/namespace":   lsNamespace,
					},
				},
			)
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

			// pod status
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

			expectedMonitoringInStatus := uniqueAssociationCount(logstash.Spec.Monitoring.Metrics.ElasticsearchRefs, logstash.Spec.Monitoring.Logs.ElasticsearchRefs)
			// monitoring status
			actualMonitoringInStatus := len(logstash.Status.MonitoringAssociationStatus)
			if expectedMonitoringInStatus != actualMonitoringInStatus {
				return fmt.Errorf("expected %d monitoring associations in status but got %d", expectedMonitoringInStatus, actualMonitoringInStatus)
			}
			for a, s := range logstash.Status.MonitoringAssociationStatus {
				if s != v1.AssociationEstablished {
					return fmt.Errorf("monitoring association %s has status %s ", a, s)
				}
			}

			// elasticsearch status
			expectedEsRefsInStatus := len(logstash.Spec.ElasticsearchRefs)
			actualEsRefsInStatus := len(logstash.Status.ElasticsearchAssociationsStatus)
			if expectedEsRefsInStatus != actualEsRefsInStatus {
				return fmt.Errorf("expected %d elasticsearch associations in status but got %d", expectedEsRefsInStatus, actualEsRefsInStatus)
			}
			for a, s := range logstash.Status.ElasticsearchAssociationsStatus {
				if s != v1.AssociationEstablished {
					return fmt.Errorf("elasticsearch association %s has status %s ", a, s)
				}
			}

			return nil
		}),
	}
}

func uniqueAssociationCount(refsList ...[]v1.ObjectSelector) int {
	uniqueAssociations := make(map[v1.ObjectSelector]struct{})
	for _, refs := range refsList {
		for _, val := range refs {
			uniqueAssociations[val] = struct{}{}
		}
	}
	return len(uniqueAssociations)
}

func (b Builder) CheckStackTestSteps(k *test.K8sClient) test.StepList {
	var username, password string

	if b.ExpectedAPIServer != nil {
		username = b.ExpectedAPIServer.Username
		password = b.ExpectedAPIServer.Password
	} else if b.Logstash.Spec.Config != nil {
		cfg := settings.MustCanonicalConfig(b.Logstash.Spec.Config.Data)
		username, _ = cfg.String("api.auth.basic.username")
		password, _ = cfg.String("api.auth.basic.password")
	}

	return test.StepList{
		b.CheckMetricsRequest(k,
			Request{
				Name:     "metrics",
				Path:     "/",
				Username: username,
				Password: password,
			},
			Want{
				MatchFunc: map[string]func(string) bool{
					"status": isGreenOrYellow,
				},
			}),
		b.CheckMetricsRequest(k,
			Request{
				Name:     "default pipeline",
				Path:     "/_node/pipelines/main",
				Username: username,
				Password: password,
			},
			Want{
				Match: map[string]string{
					"pipelines.main.batch_size": "125",
				},
				MatchFunc: map[string]func(string) bool{
					"status": isGreenOrYellow,
				},
			}),
	}
}

func (b Builder) CheckMetricsRequest(k *test.K8sClient, req Request, want Want) test.Step {
	return test.Step{
		Name: fmt.Sprintf("Logstash should respond to %s requests", req.Name),
		Test: test.Eventually(func() error {
			// send request and parse to map obj
			client, err := NewLogstashClient(b.Logstash, k)
			if err != nil {
				return err
			}

			bytes, err := DoRequest(client, b.Logstash, "GET", req.Path, req.Username, req.Password)
			if err != nil {
				return err
			}

			var response map[string]interface{}
			err = json.Unmarshal(bytes, &response)
			if err != nil {
				return err
			}

			// parse response to ucfg.Config for traverse
			res, err := settings.NewCanonicalConfigFrom(response)
			if err != nil {
				return err
			}

			// check expected string
			for k, v := range want.Match {
				str, err := res.String(k)
				if err != nil {
					return err
				}
				if str != v {
					return fmt.Errorf("expected %s to be %s but got %s", k, v, str)
				}
			}

			// check expected expression
			for k, f := range want.MatchFunc {
				str, err := res.String(k)
				if err != nil {
					return err
				}
				if !f(str) {
					return fmt.Errorf("expression failed: %s got %s", k, str)
				}
			}

			return nil
		}),
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

// isGreenOrYellow returns true if the status is either green or yellow, red is considered as failure in health API.
func isGreenOrYellow(status string) bool {
	return status == "green" || status == "yellow"
}

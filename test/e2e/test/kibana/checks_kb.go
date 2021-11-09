// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

type kbChecks struct {
	client *test.K8sClient
}

type kbStatus struct {
	Status struct {
		Overall struct {
			State string `json:"state"`
			Level string `json:"level"`
		} `json:"overall"`
	} `json:"status"`
}

func (b Builder) CheckStackTestSteps(k *test.K8sClient) test.StepList {
	if b.Kibana.Spec.Count == 0 {
		return test.StepList{}
	}

	checks := kbChecks{
		client: k,
	}
	tests := test.StepList{
		checks.CheckKbStatusHealthy(b),
	}
	if b.Kibana.Spec.EnterpriseSearchRef.IsDefined() {
		tests = append(tests, checks.CheckEntSearchAccess(b))
	}

	return tests
}

// CheckKbStatusHealthy checks that Kibana is able to connect to Elasticsearch by inspecting its API status.
func (check *kbChecks) CheckKbStatusHealthy(b Builder) test.Step {
	return test.Step{
		Name: "Kibana should be able to connect to Elasticsearch",
		Test: test.Eventually(func() error {
			password, err := check.client.GetElasticPassword(b.ElasticsearchRef().NamespacedName())
			if err != nil {
				return errors.Wrap(err, "while getting elastic password")
			}
			body, err := DoRequest(check.client, b.Kibana, password, "GET", "/api/status", nil)
			if err != nil {
				return err
			}
			var status kbStatus
			err = json.Unmarshal(body, &status)
			if err != nil {
				return err
			}

			// Starting with 8.0 the default format of /api/status response is changed. For more details see
			// https://github.com/elastic/kibana/pull/76054.
			if version.MustParse(b.Kibana.Spec.Version).LT(version.MinFor(8, 0, 0)) {
				if status.Status.Overall.State != "green" {
					return fmt.Errorf("not ready: want 'green' state but it was '%s' ", status.Status.Overall.State)
				}
			} else if status.Status.Overall.Level != "available" {
				return fmt.Errorf("not ready: want 'available' level but it was '%s'", status.Status.Overall.Level)
			}
			return nil
		}),
	}
}

// CheckEntSearchAccess checks that the Enterprise Search UI is accessible in Kibana.
func (check *kbChecks) CheckEntSearchAccess(b Builder) test.Step {
	return test.Step{
		Name: "The Enterprise Search UI should be available in Kibana",
		Test: test.Eventually(func() error {
			password, err := check.client.GetElasticPassword(b.ElasticsearchRef().NamespacedName())
			if err != nil {
				return errors.Wrap(err, "while getting elastic password")
			}
			// returns 200 OK if accessible
			path := "/api/enterprise_search/config_data"

			// new API endpoint
			if version.MustParse(b.Kibana.Spec.Version).GTE(version.MinFor(7, 16, 0)) {
				path = "/internal/workplace_search/overview"
			}
			_, err = DoRequest(check.client, b.Kibana, password, "GET", path, nil)
			return err
		}),
	}
}

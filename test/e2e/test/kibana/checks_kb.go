// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/pkg/errors"
)

type kbChecks struct {
	client *test.K8sClient
}

type kbStatus struct {
	Status struct {
		Overall struct {
			State string `json:"state"`
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
	return test.StepList{
		checks.CheckKbStatusHealthy(b),
		checks.CheckKbTelemetryStatus(b),
	}
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
			if status.Status.Overall.State != "green" {
				return fmt.Errorf("not ready: want 'green' but Kibana status was '%s'", status.Status.Overall.State)
			}
			return nil
		}),
	}
}

func (check *kbChecks) CheckKbTelemetryStatus(b Builder) test.Step {
	return test.Step{
		Name: "Kibana telemetry status should be as expected",
		Test: test.Eventually(func() error {
			shouldBeEnabled, err := kibana.IsTelemetryEnabled(b.Kibana)
			if err != nil {
				return err
			}

			_, err = MakeTelemetryRequest(b, check.client)
			switch {
			case shouldBeEnabled && err == nil:
				return nil
			case !shouldBeEnabled:
				// Kibana returns a 404 if telemetry is disabled
				if e, ok := err.(*APIError); ok && e.StatusCode == http.StatusNotFound {
					return nil
				}
			}
			return fmt.Errorf("telemetry in Kibana should be enabled [%v] but got [%s]", shouldBeEnabled, err.Error())
		}),
	}
}

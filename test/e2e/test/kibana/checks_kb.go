// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"encoding/json"
	"fmt"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
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
		checks.CheckKbStatusHealthy(b.Kibana, b.ExternalElasticsearchRef),
	}
}

// CheckKbStatusHealthy checks that Kibana is able to connect to Elasticsearch by inspecting its API status.
func (check *kbChecks) CheckKbStatusHealthy(kb kbv1.Kibana, es commonv1.ObjectSelector) test.Step {
	return test.Step{
		Name: "Kibana should be able to connect to Elasticsearch",
		Test: test.Eventually(func() error {
			if !es.IsDefined() { // if no external Elasticsearch cluster is defined, use the ElasticsearchRef
				es = kb.ElasticsearchRef().WithDefaultNamespace(kb.Namespace)
			}
			password, err := check.client.GetElasticPassword(es.NamespacedName())
			if err != nil {
				return errors.Wrap(err, "while getting elastic password")
			}
			body, err := DoRequestWithPassword(check.client, kb, password, "GET", "/api/status", nil)
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

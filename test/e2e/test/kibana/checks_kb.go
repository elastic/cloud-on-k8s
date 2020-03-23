// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"encoding/json"
	"fmt"

	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"k8s.io/apimachinery/pkg/types"
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
func (check *kbChecks) CheckKbStatusHealthy(kb kbv1.Kibana, es types.NamespacedName) test.Step {
	return test.Step{
		Name: "Kibana should be able to connect to Elasticsearch",
		Test: test.Eventually(func() error {
			if es.Name == "" { // if no external Elasticsearch cluster is used, use the ElasticsearchRef
				es = kb.ElasticsearchRef().WithDefaultNamespace(kb.Namespace).NamespacedName()
			}
			body, err := DoRequestWithES(check.client, kb, es, "GET", "/api/status", nil)
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

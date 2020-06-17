// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package beat

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/controller/beat/filebeat"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/beat"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/kibana"
)

type kbSavedObjects struct {
	Total int `json:"total"`
}

func TestBeatKibanaRef(t *testing.T) {
	name := "test-beat-kibanaref"

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	kbBuilder := kibana.NewBuilder(name).
		WithNodeCount(1).
		WithElasticsearchRef(esBuilder.Ref())

	fbBuilder := beat.NewBuilder(name).
		WithType(filebeat.Type).
		WithElasticsearchRef(esBuilder.Ref()).
		WithKibanaRef(kbBuilder.Ref())

	fbBuilder = applyYamls(t, fbBuilder, e2eFilebeatConfig, e2eFilebeatPodTemplate)

	dashboardCheck := func(client *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Verify dashboards installed",
				Test: test.Eventually(func() error {
					password, err := client.GetElasticPassword(esBuilder.Ref().NamespacedName())
					if err != nil {
						return err
					}

					body, err := kibana.DoRequest(client, kbBuilder.Kibana, password, "GET", "/api/saved_objects/_find?type=dashboard", nil)
					if err != nil {
						return err
					}
					var dashboards kbSavedObjects
					if err := json.Unmarshal(body, &dashboards); err != nil {
						return err
					}
					if dashboards.Total == 0 {
						return fmt.Errorf("expected >0 dashboards but got 0")
					}
					return nil
				}),
			},
		}
	}

	test.Sequence(nil, dashboardCheck, esBuilder, kbBuilder, fbBuilder).RunSequential(t)
}

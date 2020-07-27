// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package beat

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/beat/filebeat"
	"github.com/elastic/cloud-on-k8s/pkg/controller/beat/heartbeat"
	"github.com/elastic/cloud-on-k8s/pkg/controller/beat/metricbeat"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/beat"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/kibana"
)

type kbSavedObjects struct {
	Total        int `json:"total"`
	SavedObjects []struct {
		Attributes struct {
			Title string `json:"title"`
		} `json:"attributes"`
	} `json:"saved_objects"`
}

func (so kbSavedObjects) HasDashboardsWithPrefix(prefix string) bool {
	for _, obj := range so.SavedObjects {
		if strings.HasPrefix(obj.Attributes.Title, prefix) {
			return true
		}
	}
	return false
}

func TestBeatKibanaRefWithTLSDisabled(t *testing.T) {
	name := "test-beat-kibanaref-no-tls"

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithTLSDisabled(true)

	kbBuilder := kibana.NewBuilder(name).
		WithNodeCount(1).
		WithElasticsearchRef(esBuilder.Ref()).
		WithTLSDisabled(true)

	fbBuilder := beat.NewBuilder(name).
		WithType(filebeat.Type).
		WithRoles(beat.PSPClusterRoleName, beat.AutodiscoverClusterRoleName).
		WithElasticsearchRef(esBuilder.Ref()).
		WithKibanaRef(kbBuilder.Ref())

	fbBuilder = applyYamls(t, fbBuilder, e2eFilebeatConfig, e2eFilebeatPodTemplate)

	dashboardCheck := getDashboardCheck(
		esBuilder,
		kbBuilder,
		map[string]bool{
			"Filebeat": true,
		})

	test.Sequence(nil, dashboardCheck, esBuilder, kbBuilder, fbBuilder).RunSequential(t)
}

func TestBeatKibanaRef(t *testing.T) {
	name := "test-beat-kibanaref"

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	kbBuilder := kibana.NewBuilder(name).
		WithNodeCount(1).
		WithElasticsearchRef(esBuilder.Ref())

	fbBuilder := beat.NewBuilder(name).
		WithType(filebeat.Type).
		WithRoles(beat.PSPClusterRoleName, beat.AutodiscoverClusterRoleName).
		WithElasticsearchRef(esBuilder.Ref()).
		WithKibanaRef(kbBuilder.Ref())

	fbBuilder = applyYamls(t, fbBuilder, e2eFilebeatConfig, e2eFilebeatPodTemplate)

	mbBuilder := beat.NewBuilder(name).
		WithType(metricbeat.Type).
		WithRoles(beat.PSPClusterRoleName, beat.AutodiscoverClusterRoleName).
		WithElasticsearchRef(esBuilder.Ref()).
		WithKibanaRef(kbBuilder.Ref()).
		WithRoles(beat.MetricbeatClusterRoleName)

	mbBuilder = applyYamls(t, mbBuilder, e2eMetricbeatConfig, e2eMetricbeatPodTemplate)

	hbBuilder := beat.NewBuilder(name).
		WithType(heartbeat.Type).
		WithRoles(beat.PSPClusterRoleName).
		WithDeployment().
		WithElasticsearchRef(esBuilder.Ref())

	configYaml := fmt.Sprintf(e2eHeartBeatConfigTpl, esv1.HTTPService(esBuilder.Elasticsearch.Name), esBuilder.Elasticsearch.Namespace)

	hbBuilder = applyYamls(t, hbBuilder, configYaml, e2eHeartbeatPodTemplate)

	dashboardCheck := getDashboardCheck(
		esBuilder,
		kbBuilder,
		map[string]bool{
			"Filebeat":   true,
			"Metricbeat": true,
			"Heartbeat":  false,
		})

	test.Sequence(nil, dashboardCheck, esBuilder, kbBuilder, fbBuilder, mbBuilder, hbBuilder).RunSequential(t)
}

func getDashboardCheck(esBuilder elasticsearch.Builder, kbBuilder kibana.Builder, beatToDashboardsPresence map[string]bool) test.StepsFunc {
	return func(client *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Verify dashboards installed",
				Test: test.Eventually(func() error {
					password, err := client.GetElasticPassword(esBuilder.Ref().NamespacedName())
					if err != nil {
						return err
					}

					body, err := kibana.DoRequest(client, kbBuilder.Kibana, password,
						"GET", "/api/saved_objects/_find?type=dashboard", nil,
					)
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
					for beat, expectDashboards := range beatToDashboardsPresence {
						// We are exploiting the fact here that Beats dashboards follow a naming convention that starts with the
						// name of the beat in square brackets. This test will obviously break if future versions of Beats
						// abandon this naming convention.
						hasDashboards := dashboards.HasDashboardsWithPrefix(fmt.Sprintf("[%s ", beat))
						if hasDashboards != expectDashboards {
							return fmt.Errorf("expected  %s dashboard [%v], found dashboards [%v]", beat, expectDashboards, hasDashboards)
						}
					}
					return nil
				}),
			},
		}
	}
}

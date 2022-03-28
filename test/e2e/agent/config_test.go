// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build agent || e2e
// +build agent e2e

package agent

import (
	"fmt"
	"testing"

	"github.com/ghodss/yaml"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/agent"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/beat"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/kibana"
)

func TestSystemIntegrationConfig(t *testing.T) {
	name := "test-agent-system-int"

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	testPodBuilder := beat.NewPodBuilder(name)

	agentBuilder := agent.NewBuilder(name).
		WithRoles(agent.PSPClusterRoleName).
		WithElasticsearchRefs(agent.ToOutput(esBuilder.Ref(), "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.LogsType, "elastic_agent", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.LogsType, "elastic_agent.filebeat", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.LogsType, "elastic_agent.metricbeat", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "elastic_agent.filebeat", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "elastic_agent.metricbeat", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.cpu", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.diskio", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.load", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.memory", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.network", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.process", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.process_summary", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.socket_summary", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.uptime", "default"))

	agentBuilder = agent.ApplyYamls(t, agentBuilder, E2EAgentSystemIntegrationConfig, E2EAgentSystemIntegrationPodTemplate)

	test.Sequence(nil, test.EmptySteps, esBuilder, agentBuilder, testPodBuilder).RunSequential(t)
}

func TestAgentConfigRef(t *testing.T) {
	name := "test-agent-configref"

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	secretName := "test-agent-config"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: test.Ctx().ManagedNamespace(0),
		},
		Data: map[string][]byte{
			"agent.yml": []byte(E2EAgentSystemIntegrationConfig),
		},
	}

	agentBuilder := agent.NewBuilder(name).
		WithRoles(agent.PSPClusterRoleName).
		WithConfigRef(secretName).
		WithObjects(secret).
		WithElasticsearchRefs(agent.ToOutput(esBuilder.Ref(), "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.LogsType, "elastic_agent", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.LogsType, "elastic_agent.filebeat", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.LogsType, "elastic_agent.metricbeat", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "elastic_agent.filebeat", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "elastic_agent.metricbeat", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.cpu", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.diskio", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.load", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.memory", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.network", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.process", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.process_summary", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.socket_summary", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.uptime", "default"))

	agentBuilder = agent.ApplyYamls(t, agentBuilder, "", E2EAgentSystemIntegrationPodTemplate)

	test.Sequence(nil, test.EmptySteps, esBuilder, agentBuilder).RunSequential(t)
}

func TestMultipleOutputConfig(t *testing.T) {
	name := "test-agent-multi-out"

	esBuilder1 := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	esBuilder2 := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	agentBuilder := agent.NewBuilder(name).
		WithRoles(agent.PSPClusterRoleName).
		WithElasticsearchRefs(
			agent.ToOutput(esBuilder1.Ref(), "default"),
			agent.ToOutput(esBuilder2.Ref(), "monitoring"),
		).
		WithESValidation(agent.HasWorkingDataStream(agent.LogsType, "elastic_agent", "default"), "monitoring").
		WithESValidation(agent.HasWorkingDataStream(agent.LogsType, "elastic_agent.filebeat", "default"), "monitoring").
		WithESValidation(agent.HasWorkingDataStream(agent.LogsType, "elastic_agent.metricbeat", "default"), "monitoring").
		WithESValidation(agent.HasWorkingDataStream(agent.MetricsType, "elastic_agent.filebeat", "default"), "monitoring").
		WithESValidation(agent.HasWorkingDataStream(agent.MetricsType, "elastic_agent.metricbeat", "default"), "monitoring").
		WithESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.cpu", "default"), "default").
		WithESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.diskio", "default"), "default").
		WithESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.load", "default"), "default").
		WithESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.memory", "default"), "default").
		WithESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.network", "default"), "default").
		WithESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.process", "default"), "default").
		WithESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.process_summary", "default"), "default").
		WithESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.socket_summary", "default"), "default").
		WithESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.uptime", "default"), "default")

	agentBuilder = agent.ApplyYamls(t, agentBuilder, E2EAgentMultipleOutputConfig, E2EAgentSystemIntegrationPodTemplate)

	test.Sequence(nil, test.EmptySteps, esBuilder1, esBuilder2, agentBuilder).RunSequential(t)
}

func TestFleetMode(t *testing.T) {
	v := version.MustParse(test.Ctx().ElasticStackVersion)
	// installation of policies and integrations through Kibana file based configuration was broken between those versions:
	if v.LT(version.MinFor(8, 1, 0)) && v.GTE(version.MinFor(8, 0, 0)) {
		t.SkipNow()
	}

	name := "test-agent-fleet"

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	kbBuilder := kibana.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(1)

	fleetServerBuilder := agent.NewBuilder(name+"-fs").
		WithRoles(agent.PSPClusterRoleName, agent.AgentFleetModeRoleName).
		WithDeployment().
		WithFleetMode().
		WithFleetServer().
		WithElasticsearchRefs(agent.ToOutput(esBuilder.Ref(), "default")).
		WithKibanaRef(kbBuilder.Ref()).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.LogsType, "elastic_agent.fleet_server", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.LogsType, "elastic_agent.filebeat", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.LogsType, "elastic_agent.metricbeat", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "elastic_agent.elastic_agent", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "elastic_agent.filebeat", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "elastic_agent.metricbeat", "default"))

	kbBuilder = kbBuilder.WithConfig(fleetConfigForKibana(t, fleetServerBuilder.Agent.Spec.Version, esBuilder.Ref(), fleetServerBuilder.Ref()))

	agentBuilder := agent.NewBuilder(name+"-ea").
		WithRoles(agent.PSPClusterRoleName, agent.AgentFleetModeRoleName).
		WithFleetMode().
		WithKibanaRef(kbBuilder.Ref()).
		WithFleetServerRef(fleetServerBuilder.Ref())

	fleetServerBuilder = agent.ApplyYamls(t, fleetServerBuilder, "", E2EAgentFleetModePodTemplate)
	agentBuilder = agent.ApplyYamls(t, agentBuilder, "", E2EAgentFleetModePodTemplate)

	test.Sequence(nil, test.EmptySteps, esBuilder, kbBuilder, fleetServerBuilder, agentBuilder).RunSequential(t)
}

func fleetConfigForKibana(t *testing.T, agentVersion string, esRef v1.ObjectSelector, fsRef v1.ObjectSelector) map[string]interface{} {
	t.Helper()
	kibanaConfig := map[string]interface{}{}

	v, err := version.Parse(agentVersion)
	if err != nil {
		t.Fatalf("Unable to parse Agent version: %v", err)
	}
	if v.GTE(version.MustParse("7.16.0")) {
		// Starting with 7.16.0 we explicitly declare policies instead of relying on the default ones.
		// This is mandatory starting with 8.0.0. See https://github.com/elastic/cloud-on-k8s/issues/5262.
		if err := yaml.Unmarshal([]byte(E2EFleetPolicies), &kibanaConfig); err != nil {
			t.Fatalf("Unable to parse Fleet policies: %v", err)
		}
	}
	kibanaConfig["xpack.fleet.agents.elasticsearch.hosts"] = []string{
		fmt.Sprintf(
			"https://%s-es-http.%s.svc:9200",
			esRef.Name,
			esRef.Namespace,
		),
	}

	kibanaConfig["xpack.fleet.agents.fleet_server.hosts"] = []string{
		fmt.Sprintf(
			"https://%s-agent-http.%s.svc:8220",
			fsRef.Name,
			fsRef.Namespace,
		),
	}
	return kibanaConfig
}

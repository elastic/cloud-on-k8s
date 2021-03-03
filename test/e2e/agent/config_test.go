// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build agent e2e

package agent

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/agent"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/beat"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

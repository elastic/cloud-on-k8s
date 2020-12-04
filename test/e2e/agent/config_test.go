// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package agent

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/agent"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/beat"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
)

func TestSystemIntegrationConfig(t *testing.T) {
	name := "test-agent-system-int"

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	testPodBuilder := beat.NewPodBuilder(name)

	agentBuilder := agent.NewBuilder(name).
		WithRoles(agent.PSPClusterRoleName).
		WithElasticsearchRefs(agent.ToOutput(esBuilder.Ref(), "default")).
		WithDefaultESValidation(agent.HasEventFromAgent()).
		WithDefaultESValidation(agent.HasDataStream("logs-elastic_agent-default")).
		WithDefaultESValidation(agent.HasDataStream("logs-elastic_agent.filebeat-default")).
		WithDefaultESValidation(agent.HasDataStream("logs-elastic_agent.metricbeat-default")).
		WithDefaultESValidation(agent.HasDataStream("metrics-elastic_agent.filebeat-default")).
		WithDefaultESValidation(agent.HasDataStream("metrics-elastic_agent.metricbeat-default")).
		WithDefaultESValidation(agent.HasDataStream("metrics-system.cpu-default")).
		WithDefaultESValidation(agent.HasDataStream("metrics-system.diskio-default")).
		WithDefaultESValidation(agent.HasDataStream("metrics-system.load-default")).
		WithDefaultESValidation(agent.HasDataStream("metrics-system.memory-default")).
		WithDefaultESValidation(agent.HasDataStream("metrics-system.network-default")).
		WithDefaultESValidation(agent.HasDataStream("metrics-system.process-default")).
		WithDefaultESValidation(agent.HasDataStream("metrics-system.process_summary-default")).
		WithDefaultESValidation(agent.HasDataStream("metrics-system.socket_summary-default")).
		WithDefaultESValidation(agent.HasDataStream("metrics-system.uptime-default"))

	/* Missing:
	   logfile-system.auth-default
	   logfile-system.syslog-default
	   system/metrics-system.filesystem-default
	   system/metrics-system.fsstat-default
	*/

	agentBuilder = agent.ApplyYamls(t, agentBuilder, E2EAgentSystemIntegrationConfig, E2EAgentSystemIntegrationPodTemplate)

	test.Sequence(nil, test.EmptySteps, esBuilder, agentBuilder, testPodBuilder).RunSequential(t)
}

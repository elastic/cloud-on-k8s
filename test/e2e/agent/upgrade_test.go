// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build agent || e2e
// +build agent e2e

package agent

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/agent"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/kibana"
)

func TestAgentVersionUpgradeToLatest8x(t *testing.T) {
	srcVersion := test.Ctx().ElasticStackVersion
	dstVersion := test.LatestSnapshotVersion8x

	// TODO remove skip when https://github.com/elastic/kibana/issues/126611 is fixed
	t.SkipNow()

	test.SkipInvalidUpgrade(t, srcVersion, dstVersion)

	name := "test-agent-upgrade"
	esBuilder := elasticsearch.NewBuilder(name).
		WithVersion(srcVersion).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	kbBuilder := kibana.NewBuilder(name).
		WithVersion(srcVersion).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(1)

	fleetServerBuilder := agent.NewBuilder(name+"-fs").
		WithVersion(srcVersion).
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
		WithVersion(srcVersion).
		WithRoles(agent.PSPClusterRoleName, agent.AgentFleetModeRoleName).
		WithFleetMode().
		WithKibanaRef(kbBuilder.Ref()).
		WithFleetServerRef(fleetServerBuilder.Ref())

	fleetServerBuilder = agent.ApplyYamls(t, fleetServerBuilder, "", E2EAgentFleetModePodTemplate)
	agentBuilder = agent.ApplyYamls(t, agentBuilder, "", E2EAgentFleetModePodTemplate)

	test.RunMutations(
		t,
		[]test.Builder{esBuilder, kbBuilder, fleetServerBuilder, agentBuilder},
		[]test.Builder{
			esBuilder.WithVersion(dstVersion).WithMutatedFrom(&esBuilder),
			kbBuilder.WithVersion(dstVersion).WithMutatedFrom(&kbBuilder),
			fleetServerBuilder.WithVersion(dstVersion),
			agentBuilder.WithVersion(dstVersion),
		},
	)
}

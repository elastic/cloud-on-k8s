// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build agent || e2e

package agent

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/agent"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/kibana"
)

// TestFleetAgentWithoutTLS tests a Fleet Server, and Elastic Agent with TLS disabled for the HTTP layer.
func TestFleetAgentWithoutTLS(t *testing.T) {
	v := version.MustParse(test.Ctx().ElasticStackVersion)

	// Disabling TLS for Fleet isn't supported before 7.16, as Elasticsearch doesn't allow
	// api keys to be enabled when TLS is disabled.
	if v.LT(version.MustParse("7.16.0")) {
		t.SkipNow()
	}

	name := "test-fleet-agent-notls"
	esBuilder := elasticsearch.NewBuilder(name).
		WithVersion(v.String()).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithTLSDisabled(true)

	// Elasticsearch API keys are not automatically enabled in versions >= 7.16.0 and < 8.0.0 when TLS is disabled.
	if v.LT(version.MustParse("8.0.0")) && v.GTE(version.MustParse("7.16.0")) {
		esBuilder = esBuilder.WithAdditionalConfig(map[string]map[string]interface{}{
			"masterdata": {
				"xpack.security.authc.api_key.enabled": "true",
			},
		})
	}

	kbBuilder := kibana.NewBuilder(name).
		WithVersion(v.String()).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(1).
		WithTLSDisabled(true)

	fleetServerBuilder := agent.NewBuilder(name + "-fs").
		WithRoles(agent.AgentFleetModeRoleName).
		WithVersion(v.String()).
		WithDeployment().
		WithFleetMode().
		WithFleetServer().
		WithElasticsearchRefs(agent.ToOutput(esBuilder.Ref(), "default")).
		WithKibanaRef(kbBuilder.Ref()).
		WithTLSDisabled(true).
		WithFleetAgentDataStreamsValidation()

	kbBuilder = kbBuilder.WithConfig(fleetConfigForKibana(t, fleetServerBuilder.Agent.Spec.Version, esBuilder.Ref(), fleetServerBuilder.Ref(), false))

	agentBuilder := agent.NewBuilder(name + "-ea").
		WithRoles(agent.AgentFleetModeRoleName).
		WithVersion(v.String()).
		WithFleetMode().
		WithKibanaRef(kbBuilder.Ref()).
		WithFleetServerRef(fleetServerBuilder.Ref())

	fleetServerBuilder = agent.ApplyYamls(t, fleetServerBuilder, "", E2EAgentFleetModePodTemplate)
	agentBuilder = agent.ApplyYamls(t, agentBuilder, "", E2EAgentFleetModePodTemplate)

	test.Sequence(nil, test.EmptySteps, esBuilder, kbBuilder, fleetServerBuilder, agentBuilder).
		RunSequential(t)
}

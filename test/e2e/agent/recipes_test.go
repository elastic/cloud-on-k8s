// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build agent e2e

package agent

import (
	"path"
	"testing"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/agent/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/agent"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/helper"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestSystemIntegrationRecipe(t *testing.T) {
	customize := func(builder agent.Builder) agent.Builder {
		return builder.
			WithRoles(agent.PSPClusterRoleName).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.LogsType, "elastic_agent", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.LogsType, "elastic_agent.metricbeat", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "elastic_agent.metricbeat", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.cpu", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.diskio", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.fsstat", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.load", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.memory", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.network", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.process", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.process_summary", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.socket_summary", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.uptime", "default"))
	}

	runBeatRecipe(t, "system-integration.yaml", customize)
}

func TestKubernetesIntegrationRecipe(t *testing.T) {
	customize := func(builder agent.Builder) agent.Builder {
		return builder.
			WithRoles(agent.PSPClusterRoleName).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.LogsType, "elastic_agent", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.LogsType, "elastic_agent.metricbeat", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "elastic_agent.metricbeat", "default")).
			// TODO API server should generate event in time but on kind we see repeatedly no metrics being reported in time
			// see https://github.com/elastic/cloud-on-k8s/issues/4092
			// WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "kubernetes.apiserver", "k8s")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "kubernetes.container", "k8s")).
			// Might not generate an event in time for this check to succeed in all environments
			// WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "kubernetes.event", "k8s")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "kubernetes.node", "k8s")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "kubernetes.pod", "k8s")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "kubernetes.system", "k8s")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "kubernetes.volume", "k8s"))
	}

	runBeatRecipe(t, "kubernetes-integration.yaml", customize)
}

func runBeatRecipe(
	t *testing.T,
	fileName string,
	customize func(builder agent.Builder) agent.Builder,
	additionalObjects ...client.Object,
) {
	filePath := path.Join("../../../config/recipes/elastic-agent", fileName)
	namespace := test.Ctx().ManagedNamespace(0)
	suffix := rand.String(4)

	transformationsWrapped := func(builder test.Builder) test.Builder {
		agentBuilder, ok := builder.(agent.Builder)
		if !ok {
			return builder
		}

		// TODO: remove once https://github.com/elastic/cloud-on-k8s/issues/4092 is resolved
		if test.Ctx().HasTag("ipv6") {
			t.SkipNow()
		}

		if isStackIncompatible(agentBuilder.Agent) {
			t.SkipNow()
		}

		// OpenShift requires different securityContext than provided in the recipe.
		// Skipping it altogether to reduce maintenance burden.
		if test.Ctx().Provider == "ocp" {
			t.SkipNow()
		}

		agentBuilder.Suffix = suffix

		if customize != nil {
			agentBuilder = customize(agentBuilder)
		}

		return agentBuilder
	}

	helper.RunFile(t, filePath, namespace, suffix, additionalObjects, transformationsWrapped)
}

// isStackIncompatible returns true iff Agent version is higher than tested Stack version
func isStackIncompatible(agent agentv1alpha1.Agent) bool {
	stackVersion := version.MustParse(test.Ctx().ElasticStackVersion)
	agentVersion := version.MustParse(agent.Spec.Version)
	return agentVersion.IsAfter(stackVersion)
}

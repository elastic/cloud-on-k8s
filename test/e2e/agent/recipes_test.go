// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build agent || e2e

package agent

import (
	"path"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/agent/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/agent"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/beat"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/helper"
)

func TestSystemIntegrationRecipe(t *testing.T) {
	customize := func(builder agent.Builder) agent.Builder {
		return builder.
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.LogsType, "elastic_agent", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.LogsType, "elastic_agent.metricbeat", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "elastic_agent.metricbeat", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.cpu", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.diskio", "default")).
			// to be reinstated once https://github.com/elastic/beats/issues/30590 is addressed
			// WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.fsstat", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.load", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.memory", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.network", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.process", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.process_summary", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.socket_summary", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.uptime", "default"))
	}

	runAgentRecipe(t, "system-integration.yaml", customize)
}

func TestKubernetesIntegrationRecipe(t *testing.T) {
	customize := func(builder agent.Builder) agent.Builder {
		return builder.
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

	runAgentRecipe(t, "kubernetes-integration.yaml", customize)
}

func TestMultiOutputRecipe(t *testing.T) {
	customize := func(builder agent.Builder) agent.Builder {
		return builder.
			WithESValidation(agent.HasWorkingDataStream(agent.LogsType, "elastic_agent", "default"), "monitoring").
			WithESValidation(agent.HasWorkingDataStream(agent.LogsType, "elastic_agent.metricbeat", "default"), "monitoring").
			WithESValidation(agent.HasWorkingDataStream(agent.MetricsType, "elastic_agent.metricbeat", "default"), "monitoring").
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.cpu", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.diskio", "default")).
			// to be reinstated once https://github.com/elastic/beats/issues/30590 is addressed
			// WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.fsstat", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.load", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.memory", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.network", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.process", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.process_summary", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.socket_summary", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.uptime", "default"))
	}

	runAgentRecipe(t, "multi-output.yaml", customize)
}

func TestFleetKubernetesIntegrationRecipe(t *testing.T) {
	customize := func(builder agent.Builder) agent.Builder {
		if !builder.Agent.Spec.FleetServerEnabled {
			return builder
		}

		builder = builder.
			WithFleetAgentDataStreamsValidation().
			// TODO API server should generate event in time but on kind we see repeatedly no metrics being reported in time
			// see https://github.com/elastic/cloud-on-k8s/issues/4092
			// WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "kubernetes.apiserver", "k8s")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "kubernetes.container", "default")).
			// Might not generate an event in time for this check to succeed in all environments
			// WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "kubernetes.event", "k8s")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "kubernetes.node", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "kubernetes.pod", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "kubernetes.proxy", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "kubernetes.system", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "kubernetes.volume", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.cpu", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.diskio", "default")).
			// to be reinstated once https://github.com/elastic/beats/issues/30590 is addressed
			// WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.fsstat", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.load", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.memory", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.network", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.process", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.process.summary", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.socket_summary", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.uptime", "default"))

		return builder
	}

	runAgentRecipe(t, "fleet-kubernetes-integration.yaml", customize)
}

func TestFleetKubernetesNonRootIntegrationRecipe(t *testing.T) {
	v := version.MustParse(test.Ctx().ElasticStackVersion)

	// https://github.com/elastic/cloud-on-k8s/issues/6331
	if v.LT(version.MinFor(8, 7, 0)) && v.GE(version.MinFor(8, 6, 0)) {
		t.SkipNow()
	}

	// The recipe does not work fully within an openshift cluster without modifications.
	if test.Ctx().OcpCluster {
		t.SkipNow()
	}

	customize := func(builder agent.Builder) agent.Builder {
		if !builder.Agent.Spec.FleetServerEnabled {
			return builder
		}

		return builder.
			WithFleetAgentDataStreamsValidation().
			// TODO API server should generate event in time but on kind we see repeatedly no metrics being reported in time
			// see https://github.com/elastic/cloud-on-k8s/issues/4092
			// WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "kubernetes.apiserver", "k8s")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "kubernetes.container", "default")).
			// Might not generate an event in time for this check to succeed in all environments
			// WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "kubernetes.event", "k8s")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "kubernetes.node", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "kubernetes.pod", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "kubernetes.proxy", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "kubernetes.system", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "kubernetes.volume", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.cpu", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.diskio", "default")).
			// to be reinstated once https://github.com/elastic/beats/issues/30590 is addressed
			// WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.fsstat", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.load", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.memory", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.network", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.process", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.process.summary", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.socket_summary", "default")).
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.uptime", "default"))
	}

	runAgentRecipe(t, "fleet-kubernetes-integration-nonroot.yaml", customize)
}

func TestFleetCustomLogsIntegrationRecipe(t *testing.T) {
	v := version.MustParse(test.Ctx().ElasticStackVersion)

	if v.GT(version.MinFor(8, 8, 0)) {
		t.Skip("Disabled since 8.8.0, refer to https://github.com/elastic/cloud-on-k8s/issues/5105")
	}

	notLoggingPod := beat.NewPodBuilder("test")
	loggingPod := beat.NewPodBuilder("test")
	loggingPod.Pod.Namespace = "default"

	customize := func(builder agent.Builder) agent.Builder {
		if !builder.Agent.Spec.FleetServerEnabled {
			return builder
		}

		return builder.
			WithDefaultESValidation(agent.HasWorkingDataStream(agent.LogsType, "generic", "default")).
			WithFleetAgentDataStreamsValidation().
			WithDefaultESValidation(agent.HasEvent("/_search?q=message:" + loggingPod.Logged)).
			WithDefaultESValidation(agent.NoEvent("/_search?q=message:" + notLoggingPod.Logged))
	}

	runAgentRecipe(t, "fleet-custom-logs-integration.yaml", customize, &loggingPod.Pod, &notLoggingPod.Pod)
}

func TestFleetAPMIntegrationRecipe(t *testing.T) {
	customize := func(builder agent.Builder) agent.Builder {
		if !builder.Agent.Spec.FleetServerEnabled {
			return builder
		}
		return builder.WithFleetAgentDataStreamsValidation()
	}
	runAgentRecipe(t, "fleet-apm-integration.yaml", customize)
}

func runAgentRecipe(
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
		if strings.HasPrefix(test.Ctx().Provider, "ocp") {
			t.SkipNow()
		}

		agentBuilder.Suffix = suffix

		if customize != nil {
			agentBuilder = customize(agentBuilder)
		}

		agentBuilder = agentBuilder.MoreResourcesForIssue4730()
		return agentBuilder
	}

	helper.RunFile(t, filePath, namespace, suffix, additionalObjects, transformationsWrapped)
}

// isStackIncompatible returns true iff Agent version is higher than tested Stack version
func isStackIncompatible(agent agentv1alpha1.Agent) bool {
	stackVersion := version.MustParse(test.Ctx().ElasticStackVersion)
	agentVersion := version.MustParse(agent.Spec.Version)
	return agentVersion.GT(stackVersion)
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// +build beat e2e

package beat

import (
	"fmt"
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/beat/auditbeat"
	"github.com/elastic/cloud-on-k8s/pkg/controller/beat/filebeat"
	"github.com/elastic/cloud-on-k8s/pkg/controller/beat/heartbeat"
	"github.com/elastic/cloud-on-k8s/pkg/controller/beat/journalbeat"
	"github.com/elastic/cloud-on-k8s/pkg/controller/beat/metricbeat"
	"github.com/elastic/cloud-on-k8s/pkg/controller/beat/packetbeat"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/beat"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/kibana"
)

func TestFilebeatDefaultConfig(t *testing.T) {
	name := "test-fb-default-cfg"

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	testPodBuilder := beat.NewPodBuilder(name)

	fbBuilder := beat.NewBuilder(name).
		WithRoles(beat.PSPClusterRoleName, beat.AutodiscoverClusterRoleName).
		WithType(filebeat.Type).
		WithElasticsearchRef(esBuilder.Ref()).
		WithESValidations(
			beat.HasEventFromBeat(filebeat.Type),
			beat.HasEventFromPod(testPodBuilder.Pod.Name),
			beat.HasMessageContaining(testPodBuilder.Logged))

	fbBuilder = beat.ApplyYamls(t, fbBuilder, E2EFilebeatConfig, E2EFilebeatPodTemplate)

	test.Sequence(nil, test.EmptySteps, esBuilder, fbBuilder, testPodBuilder).RunSequential(t)
}

func TestMetricbeatDefaultConfig(t *testing.T) {
	name := "test-mb-default-cfg"

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	testPodBuilder := beat.NewPodBuilder(name)

	mbBuilder := beat.NewBuilder(name).
		WithType(metricbeat.Type).
		WithRoles(beat.MetricbeatClusterRoleName, beat.PSPClusterRoleName, beat.AutodiscoverClusterRoleName).
		WithElasticsearchRef(esBuilder.Ref()).
		WithESValidations(
			beat.HasEventFromBeat(metricbeat.Type),
			beat.HasEvent("event.dataset:system.cpu"),
			beat.HasEvent("event.dataset:system.load"),
			beat.HasEvent("event.dataset:system.memory"),
			beat.HasEvent("event.dataset:system.network"),
			beat.HasEvent("event.dataset:system.process"),
			beat.HasEvent("event.dataset:system.process.summary"),
			beat.HasEvent("event.dataset:system.fsstat"),
		)

	mbBuilder = beat.ApplyYamls(t, mbBuilder, e2eMetricbeatConfig, e2eMetricbeatPodTemplate)

	test.Sequence(nil, test.EmptySteps, esBuilder, mbBuilder, testPodBuilder).RunSequential(t)
}

func TestHeartbeatConfig(t *testing.T) {
	name := "test-hb-cfg"

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	hbBuilder := beat.NewBuilder(name).
		WithType(heartbeat.Type).
		WithRoles(beat.PSPClusterRoleName).
		WithDeployment().
		WithElasticsearchRef(esBuilder.Ref()).
		WithESValidations(
			beat.HasEventFromBeat(heartbeat.Type),
			beat.HasEvent("monitor.status:up"))

	configYaml := fmt.Sprintf(e2eHeartBeatConfigTpl, v1.HTTPService(esBuilder.Elasticsearch.Name), esBuilder.Elasticsearch.Namespace)

	hbBuilder = beat.ApplyYamls(t, hbBuilder, configYaml, e2eHeartbeatPodTemplate)

	test.Sequence(nil, test.EmptySteps, esBuilder, hbBuilder).RunSequential(t)
}

func TestBeatSecureSettings(t *testing.T) {
	name := "test-beat-secure-settings"

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	testPodBuilder := beat.NewPodBuilder(name)

	secretName := "secret-agent"
	agentName := "test-agent-name-xyz"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: test.Ctx().ManagedNamespace(0),
		},
		Data: map[string][]byte{
			"AGENT_NAME_VAR": []byte(agentName),
		},
	}

	fbBuilder := beat.NewBuilder(name).
		WithType(filebeat.Type).
		WithRoles(beat.PSPClusterRoleName, beat.AutodiscoverClusterRoleName).
		WithElasticsearchRef(esBuilder.Ref()).
		WithSecureSettings(secretName).
		WithObjects(secret).
		WithESValidations(
			beat.HasEventFromBeat(filebeat.Type),
			beat.HasEventFromPod(testPodBuilder.Pod.Name),
			beat.HasMessageContaining(testPodBuilder.Logged),
			beat.HasEvent("agent.name:"+agentName),
		)

	config := `
name: ${AGENT_NAME_VAR}
filebeat:
  autodiscover:
    providers:
    - hints:
        default_config:
          paths:
          - /var/log/containers/*${data.kubernetes.container.id}.log
          type: container
        enabled: true
      node: ${NODE_NAME}
      type: kubernetes
processors:
- add_cloud_metadata: {}
- add_host_metadata: {}
`

	fbBuilder = beat.ApplyYamls(t, fbBuilder, config, E2EFilebeatPodTemplate)

	test.Sequence(nil, test.EmptySteps, esBuilder, fbBuilder, testPodBuilder).RunSequential(t)
}

func TestBeatConfigRef(t *testing.T) {
	name := "test-beat-configref"

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	secretName := "fb-config" // nolint:gosec
	agentName := "configref-test-agent"
	config := fmt.Sprintf(`
name: %s
filebeat:
  autodiscover:
    providers:
    - hints:
        default_config:
          paths:
          - /var/log/containers/*${data.kubernetes.container.id}.log
          type: container
        enabled: true
      host: ${NODE_NAME}
      type: kubernetes
processors:
- add_cloud_metadata: {}
- add_host_metadata: {}
`, agentName)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: test.Ctx().ManagedNamespace(0),
		},
		Data: map[string][]byte{
			"beat.yml": []byte(config),
		},
	}

	fbBuilder := beat.NewBuilder(name).
		WithType(filebeat.Type).
		WithRoles(beat.PSPClusterRoleName, beat.AutodiscoverClusterRoleName).
		WithElasticsearchRef(esBuilder.Ref()).
		WithConfigRef(secretName).
		WithObjects(secret).
		WithESValidations(
			beat.HasEventFromBeat(filebeat.Type),
			beat.HasEvent("agent.name:"+agentName),
		)

	fbBuilder = beat.ApplyYamls(t, fbBuilder, "", E2EFilebeatPodTemplate)

	test.Sequence(nil, test.EmptySteps, esBuilder, fbBuilder).RunSequential(t)
}

func TestAuditbeatConfig(t *testing.T) {
	if test.Ctx().Provider == "kind" {
		// kind doesn't support configuring required settings
		// see https://github.com/elastic/cloud-on-k8s/issues/3328 for more context
		t.SkipNow()
	}

	name := "test-ab-cfg"

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	kbBuilder := kibana.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(1)

	abBuilder := beat.NewBuilder(name).
		WithType(auditbeat.Type).
		WithKibanaRef(kbBuilder.Ref()).
		WithRoles(beat.AuditbeatPSPClusterRoleName, beat.AutodiscoverClusterRoleName).
		WithElasticsearchRef(esBuilder.Ref()).
		WithESValidations(
			beat.HasEventFromBeat(auditbeat.Type),
			beat.HasEvent("event.dataset:file"),
			beat.HasEvent("event.module:file_integrity"),
		)

	abBuilder = beat.ApplyYamls(t, abBuilder, e2eAuditbeatConfig, e2eAuditbeatPodTemplate)

	test.Sequence(nil, test.EmptySteps, esBuilder, kbBuilder, abBuilder).RunSequential(t)
}

func TestPacketbeatConfig(t *testing.T) {
	name := "test-pb-cfg"

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	kbBuilder := kibana.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(1)

	pbBuilder := beat.NewBuilder(name).
		WithType(packetbeat.Type).
		WithKibanaRef(kbBuilder.Ref()).
		WithRoles(beat.PacketbeatPSPClusterRoleName, beat.AutodiscoverClusterRoleName).
		WithElasticsearchRef(esBuilder.Ref()).
		WithESValidations(
			beat.HasEventFromBeat(packetbeat.Type),
			beat.HasEvent("event.dataset:flow"),
			beat.HasEvent("event.dataset:dns"),
		)

	if !(test.Ctx().Provider == "kind" && test.Ctx().KubernetesMajorMinor() == "1.12") {
		// there are some issues with kind 1.12 and tracking http traffic
		pbBuilder = pbBuilder.WithESValidations(beat.HasEvent("event.dataset:http"))
	}

	pbBuilder = beat.ApplyYamls(t, pbBuilder, e2ePacketbeatConfig, e2ePacketbeatPodTemplate)

	test.Sequence(nil, test.EmptySteps, esBuilder, kbBuilder, pbBuilder).RunSequential(t)
}

func TestJournalbeatConfig(t *testing.T) {
	// Journalbeat was removed in 7.16
	if version.MustParse(test.Ctx().ElasticStackVersion).GTE(version.MinFor(7, 16, 0)) {
		t.SkipNow()
	}

	name := "test-jb-cfg"

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	jbBuilder := beat.NewBuilder(name).
		WithType(journalbeat.Type).
		WithRoles(beat.JournalbeatPSPClusterRoleName, beat.AutodiscoverClusterRoleName).
		WithElasticsearchRef(esBuilder.Ref()).
		WithESValidations(
			beat.HasEventFromBeat(journalbeat.Type),
		)

	jbBuilder = beat.ApplyYamls(t, jbBuilder, e2eJournalbeatConfig, e2eJournalbeatPodTemplate)

	test.Sequence(nil, test.EmptySteps, esBuilder, jbBuilder).RunSequential(t)
}

// --- helpers

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package beat

import (
	"fmt"
	"testing"

	ghodssyaml "github.com/ghodss/yaml"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	v1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/beat/filebeat"
	"github.com/elastic/cloud-on-k8s/pkg/controller/beat/metricbeat"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/beat"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
)

func TestFilebeatDefaultConfig(t *testing.T) {
	name := "test-fb-default-cfg"

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	testPodBuilder := beat.NewPodBuilder(name)

	fbBuilder := beat.NewBuilder(name).
		WithType(filebeat.Type).
		WithElasticsearchRef(esBuilder.Ref()).
		WithESValidations(
			beat.HasEventFromBeat(filebeat.Type),
			beat.HasEventFromPod(testPodBuilder.Pod.Name),
			beat.HasMessageContaining(testPodBuilder.Logged))

	fbBuilder = applyYamls(t, fbBuilder, e2eFilebeatConfig, e2eFilebeatPodTemplate)

	test.Sequence(nil, test.EmptySteps, esBuilder, fbBuilder, testPodBuilder).RunSequential(t)
}

func TestMetricbeatDefaultConfig(t *testing.T) {
	name := "test-mb-default-cfg"

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	testPodBuilder := beat.NewPodBuilder(name)

	mbBuilder := beat.NewBuilder(name).
		WithType(metricbeat.Type).
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

	mbBuilder = applyYamls(t, mbBuilder, e2eMetricbeatConfig, e2eMetricbeatPodTemplate)

	test.Sequence(nil, test.EmptySteps, esBuilder, mbBuilder, testPodBuilder).RunSequential(t)
}

func TestHeartbeatConfig(t *testing.T) {
	name := "test-hb-cfg"

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	hbBuilder := beat.NewBuilder(name).
		WithType("heartbeat").
		WithDeployment().
		WithElasticsearchRef(esBuilder.Ref()).
		WithImage("docker.elastic.co/beats/heartbeat:7.7.0").
		WithESValidations(
			beat.HasEventFromBeat("heartbeat"),
			beat.HasEvent("monitor.status:up"))

	podTemplateYaml := `spec:
  dnsPolicy: ClusterFirstWithHostNet
  hostNetwork: true
  securityContext:
    runAsUser: 0
`

	configYaml := fmt.Sprintf(`
heartbeat.monitors:
- type: tcp
  schedule: '@every 5s'
  hosts: ["%s.%s.svc:9200"]
`, v1.HTTPService(esBuilder.Elasticsearch.Name), esBuilder.Elasticsearch.Namespace)

	hbBuilder = applyYamls(t, hbBuilder, configYaml, podTemplateYaml)

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
      host: ${HOSTNAME}
      type: kubernetes
processors:
- add_cloud_metadata: {}
- add_host_metadata: {}
`

	fbBuilder = applyYamls(t, fbBuilder, config, e2eFilebeatPodTemplate)

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
      host: ${HOSTNAME}
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
		WithElasticsearchRef(esBuilder.Ref()).
		WithConfigRef(secretName).
		WithObjects(secret).
		WithESValidations(
			beat.HasEventFromBeat(filebeat.Type),
			beat.HasEvent("agent.name:"+agentName),
		)

	fbBuilder = applyYamls(t, fbBuilder, "", e2eFilebeatPodTemplate)

	test.Sequence(nil, test.EmptySteps, esBuilder, fbBuilder).RunSequential(t)
}

// --- helpers

func applyYamls(t *testing.T, b beat.Builder, configYaml, podTemplateYaml string) beat.Builder {
	if configYaml != "" {
		b.Beat.Spec.Config = &commonv1.Config{}
		err := settings.MustParseConfig([]byte(configYaml)).Unpack(&b.Beat.Spec.Config.Data)
		require.NoError(t, err)
	}

	if podTemplateYaml != "" {
		// use ghodss as settings package has issues with unpacking volumes part of the yamls
		err := ghodssyaml.Unmarshal([]byte(podTemplateYaml), b.PodTemplate)
		require.NoError(t, err)
	}

	return b
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package beat

import (
	"fmt"
	"testing"

	v1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/beat/filebeat"
	"github.com/elastic/cloud-on-k8s/pkg/controller/beat/metricbeat"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/beat"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
)

func TestFilebeatAutodiscoverPreset(t *testing.T) {
	name := "test-fb-ad-preset"

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	testPodBuilder := beat.NewPodBuilder(name)

	fbBuilder := beat.NewBuilder(name, filebeat.Type).
		WithPreset(v1beta1.FilebeatK8sAutodiscoverPreset).
		WithElasticsearchRef(esBuilder.Ref()).
		WithESValidations(
			beat.HasEventFromBeat(filebeat.Type),
			beat.HasEventFromPod(testPodBuilder.Pod.Name),
			beat.HasMessageContaining(testPodBuilder.Logged))

	test.Sequence(nil, test.EmptySteps, esBuilder, fbBuilder, testPodBuilder).RunSequential(t)
}

func TestMetricbeatHostsPreset(t *testing.T) {
	name := "test-mb-hosts-preset"

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	testPodBuilder := beat.NewPodBuilder(name)

	mbBuilder := beat.NewBuilder(name, metricbeat.Type).
		WithPreset(v1beta1.MetricbeatK8sHostsPreset).
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
			beat.HasEvent("event.dataset:kubernetes.pod"),
			beat.HasEvent("event.dataset:kubernetes.node"),
			beat.HasEvent("event.dataset:kubernetes.container"),
			beat.HasEvent("event.dataset:kubernetes.volume"),
			beat.HasEvent("event.dataset:kubernetes.system"),
			beat.HasEvent("kubernetes.container.name:"+beat.TestPodContainerName),
		)

	test.Sequence(nil, test.EmptySteps, esBuilder, mbBuilder, testPodBuilder).RunSequential(t)
}

func TestHeartbeatConfig(t *testing.T) {
	name := "test-hb-cfg"

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	hbBuilder := beat.NewBuilder(name, "heartbeat").
		WithElasticsearchRef(esBuilder.Ref()).
		WithImage("docker.elastic.co/beats/heartbeat:7.7.0").
		WithESValidations(
			beat.HasEventFromBeat("heartbeat"),
			beat.HasEvent("monitor.status:up"))

	hbBuilder.Beat.Spec.Deployment = &v1beta1.DeploymentSpec{}
	hbBuilder.WithSecurityContext(corev1.PodSecurityContext{
		FSGroup:      pointer.Int64(1000),
		RunAsUser:    pointer.Int64(1000),
		RunAsNonRoot: test.BoolPtr(true),
	})

	yaml := fmt.Sprintf(`
heartbeat.monitors:
- type: tcp
  schedule: '@every 5s'
  hosts: ["%s.%s.svc:9200"]
`, v1.HTTPService(esBuilder.Elasticsearch.Name), esBuilder.Elasticsearch.Namespace)
	hbBuilder = applyConfigYaml(t, hbBuilder, yaml)

	test.Sequence(nil, test.EmptySteps, esBuilder, hbBuilder).RunSequential(t)
}

// --- helpers

func applyConfigYaml(t *testing.T, b beat.Builder, yaml string) beat.Builder {
	config := &commonv1.Config{}
	err := settings.MustParseConfig([]byte(yaml)).Unpack(&config.Data)
	require.NoError(t, err)

	return b.WithConfig(config)
}

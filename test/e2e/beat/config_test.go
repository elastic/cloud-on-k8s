// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package beat

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
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

	fbBuilder := beat.NewBuilder(name, filebeat.Type).
		WithElasticsearchRef(esBuilder.Ref()).
		WithESValidations(beat.HasEventFromPod(testPodBuilder.Pod.Name))

	test.Sequence(nil, test.EmptySteps, esBuilder, fbBuilder, testPodBuilder).RunSequential(t)
}

func TestMetricbeatDefaultConfig(t *testing.T) {
	name := "test-mb-default-cfg"

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	testPodBuilder := beat.NewPodBuilder(name)

	mbBuilder := beat.NewBuilder(name, metricbeat.Type).
		WithElasticsearchRef(esBuilder.Ref()).
		WithESValidations(beat.HasEventFromBeat(metricbeat.Type))

	test.Sequence(nil, test.EmptySteps, esBuilder, mbBuilder, testPodBuilder).RunSequential(t)
}

func TestHeartbeatConfig(t *testing.T) {
	name := "test-hb-cfg"

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	hbBuilder := beat.NewBuilder(name, "heartbeat").
		WithElasticsearchRef(esBuilder.Ref()).
		WithImage("docker.elastic.co/beats/heartbeat:7.7.0").
		WithESValidations(beat.HasEventFromBeat("heartbeat"))

	yaml := fmt.Sprintf(`
heartbeat.monitors:
- type: tcp
  schedule: '@every 5s'
  hosts: ["%s.%s.svc:9200"]
`, esBuilder.Elasticsearch.Name, esBuilder.Elasticsearch.Namespace)
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

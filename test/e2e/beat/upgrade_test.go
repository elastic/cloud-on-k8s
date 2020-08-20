// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package beat

import (
	"testing"

	beatcommon "github.com/elastic/cloud-on-k8s/pkg/controller/beat/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/beat/filebeat"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/beat"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestVersionUpgradeToLatest7x(t *testing.T) {
	srcVersion := test.Ctx().ElasticStackVersion
	dstVersion := test.LatestVersion7x

	test.SkipInvalidUpgrade(t, srcVersion, dstVersion)

	name := "test-beat-upgrade-to-7x"
	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithVersion(dstVersion)

	fbBuilder := beat.NewBuilder(name).
		WithRoles(beat.PSPClusterRoleName, beat.AutodiscoverClusterRoleName).
		WithType(filebeat.Type).
		WithDeployment().
		WithDeploymentStrategy(appsv1.DeploymentStrategy{
			Type: appsv1.RecreateDeploymentStrategyType,
		}).
		WithElasticsearchRef(esBuilder.Ref())

	opts := []client.ListOption{
		client.InNamespace(fbBuilder.Beat.Namespace),
		client.MatchingLabels(map[string]string{
			common.TypeLabelName:     beatcommon.TypeLabelValue,
			beatcommon.NameLabelName: fbBuilder.Beat.Name,
		}),
	}

	fbBuilder = beat.ApplyYamls(t, fbBuilder, E2EFilebeatConfig, E2EFilebeatPodTemplate)

	test.RunMutationsWhileWatching(
		t,
		[]test.Builder{esBuilder, fbBuilder},
		[]test.Builder{esBuilder, fbBuilder.WithVersion(dstVersion)},
		[]test.Watcher{test.NewVersionWatcher(beatcommon.VersionLabelName, opts...)},
	)
}

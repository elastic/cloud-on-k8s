// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build beat || e2e

package beat

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	beatcommon "github.com/elastic/cloud-on-k8s/v2/pkg/controller/beat/common"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/beat/filebeat"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/beat"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/elasticsearch"
)

// TestBeatVersionUpgradeToLatest7x tests a version upgrade from the current e2e stack version to the latest 7.x
// while using a custom deployment strategy of type "recreate". This is to ensure that Beats pods don't run concurrently.
// If using a shared data directory on the host a replacement pod might otherwise never reach the ready state as the
// data directory stays locked it happens to be scheduled on the same node.
func TestBeatVersionUpgradeToLatest7x(t *testing.T) {
	srcVersion := test.Ctx().ElasticStackVersion
	dstVersion := test.LatestReleasedVersion7x

	test.SkipInvalidUpgrade(t, srcVersion, dstVersion)

	name := "test-beat-upgrade-to-7x"
	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithVersion(dstVersion)

	fbBuilder := beat.NewBuilder(name).
		WithRoles(beat.AutodiscoverClusterRoleName).
		WithType(filebeat.Type).
		WithDeploymentStrategy(appsv1.DeploymentStrategy{
			Type: appsv1.RecreateDeploymentStrategyType,
		}).
		WithElasticsearchRef(esBuilder.Ref())

	opts := []client.ListOption{
		client.InNamespace(fbBuilder.Beat.Namespace),
		client.MatchingLabels(map[string]string{
			commonv1.TypeLabelName:   beatcommon.TypeLabelValue,
			beatcommon.NameLabelName: fbBuilder.Beat.Name,
		}),
	}

	fbBuilder = beat.ApplyYamls(t, fbBuilder, E2EFilebeatConfig, E2EFilebeatPodTemplate)

	test.RunMutationsWhileWatching(
		t,
		[]test.Builder{esBuilder, fbBuilder},
		[]test.Builder{esBuilder, fbBuilder.WithVersion(dstVersion).WithMutatedFrom(&fbBuilder)},
		// check that only one version of Beats is running at any given time to verify that the "recreate" deployment
		// strategy has been configured successfully.
		[]test.Watcher{test.NewVersionWatcher(beatcommon.VersionLabelName, opts...)},
	)
}

func TestVersionUpgradeToLatest8x(t *testing.T) {
	srcVersion, dstVersion := test.GetUpgradePathTo8x(test.Ctx().ElasticStackVersion)

	test.SkipInvalidUpgrade(t, srcVersion, dstVersion)

	name := "test-beat-upgrade-to-8x"
	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithVersion(dstVersion)

	fbBuilder := beat.NewBuilder(name).
		WithRoles(beat.AutodiscoverClusterRoleName).
		WithType(filebeat.Type).
		WithDeploymentStrategy(appsv1.DeploymentStrategy{
			Type: appsv1.RecreateDeploymentStrategyType,
		}).
		WithElasticsearchRef(esBuilder.Ref())

	opts := []client.ListOption{
		client.InNamespace(fbBuilder.Beat.Namespace),
		client.MatchingLabels(map[string]string{
			commonv1.TypeLabelName:   beatcommon.TypeLabelValue,
			beatcommon.NameLabelName: fbBuilder.Beat.Name,
		}),
	}

	fbBuilder = beat.ApplyYamls(t, fbBuilder, E2EFilebeatConfig, E2EFilebeatPodTemplate)

	test.RunMutationsWhileWatching(
		t,
		[]test.Builder{esBuilder, fbBuilder},
		[]test.Builder{esBuilder, fbBuilder.WithVersion(dstVersion).WithMutatedFrom(&fbBuilder)},
		// check that only one version of Beats is running at any given time to verify that the "recreate" deployment
		// strategy has been configured successfully.
		[]test.Watcher{test.NewVersionWatcher(beatcommon.VersionLabelName, opts...)},
	)
}

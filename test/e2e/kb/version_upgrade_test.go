// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// +build kb e2e

package kb

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/reconcile"
	kibana2 "github.com/elastic/cloud-on-k8s/pkg/controller/kibana"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/kibana"
)

func TestVersionUpgradeToLatest7x(t *testing.T) {
	srcVersion := test.Ctx().ElasticStackVersion
	dstVersion := test.LatestReleasedVersion7x

	test.SkipInvalidUpgrade(t, srcVersion, dstVersion)

	name := "test-version-upgrade-to-7x"
	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithVersion(dstVersion)

	srcNodeCount := 3
	if srcVersion == "7.1.1" {
		// workaround for https://github.com/elastic/kibana/pull/37674 to avoid accidental .kibana index creation
		// can be removed once we stop supporting 7.1.1
		srcNodeCount = 1
	}

	kbBuilder := kibana.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(srcNodeCount).
		WithVersion(srcVersion)

	opts := []client.ListOption{
		client.InNamespace(kbBuilder.Kibana.Namespace),
		client.MatchingLabels(map[string]string{
			common.TypeLabelName:        kibana2.Type,
			kibana2.KibanaNameLabelName: kbBuilder.Kibana.Name,
		}),
	}

	// perform a Kibana version upgrade and assert that:
	// - there was a time were no Kibana pods were ready (when all old version pods were termintated,
	//   but before new version pods were started), and
	// - at all times all pods had the same Kibana version.
	test.RunMutationsWhileWatching(
		t,
		[]test.Builder{esBuilder, kbBuilder},
		[]test.Builder{esBuilder, kbBuilder.WithVersion(dstVersion).WithNodeCount(3).WithMutatedFrom(&kbBuilder)},
		[]test.Watcher{NewReadinessWatcher(opts...), test.NewVersionWatcher(kibana2.KibanaVersionLabelName, opts...)},
	)
}

func TestVersionUpgradeAndRespecToLatest7x(t *testing.T) {
	srcVersion := test.Ctx().ElasticStackVersion
	dstVersion := test.LatestReleasedVersion7x

	test.SkipInvalidUpgrade(t, srcVersion, dstVersion)

	name := "test-upgrade-and-respec-to-7x"
	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithVersion(dstVersion)

	srcNodeCount := 3
	if srcVersion == "7.1.1" {
		// workaround for https://github.com/elastic/kibana/pull/37674 to avoid accidental .kibana index creation
		// can be removed once we stop supporting 7.1.1
		srcNodeCount = 1
	}

	kbBuilder1 := kibana.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(srcNodeCount).
		WithVersion(srcVersion)

	// perform a Kibana version upgrade immediately followed by a Kibana configuration change.
	// we want to make sure that the second upgrade will be done in rolling upgrade fashion instead of terminating
	// and recreating all the pods at once.
	kbBuilder2 := kbBuilder1.WithMutatedFrom(&kbBuilder1).WithVersion(dstVersion)
	kbBuilder3 := kbBuilder2.WithMutatedFrom(&kbBuilder2).WithLabel("some", "label")

	opts := []client.ListOption{
		client.InNamespace(kbBuilder1.Kibana.Namespace),
		client.MatchingLabels(map[string]string{
			common.TypeLabelName:        kibana2.Type,
			kibana2.KibanaNameLabelName: kbBuilder1.Kibana.Name,
		}),
	}

	// checks whether after temporary downtime kibana will be available the rest of the time
	var hadZero, shouldHaveNonZero, failed bool
	w := test.NewWatcher(
		"watch pods readiness: expect some downtime once and then no downtime",
		1*time.Second,
		func(k *test.K8sClient, t *testing.T) {
			pods, err := k.GetPods(opts...)
			if err != nil {
				t.Logf("got error: %v", err)
			}

			ready := len(reconcile.AvailableElasticsearchNodes(pods))
			hadZero = hadZero || ready == 0
			if hadZero && ready > 0 {
				shouldHaveNonZero = true
			}

			if shouldHaveNonZero && ready == 0 {
				failed = true
			}
		},
		func(k *test.K8sClient, t *testing.T) {
			assert.False(t, failed)
		})

	test.RunMutationsWhileWatching(
		t,
		[]test.Builder{esBuilder, kbBuilder1},
		[]test.Builder{esBuilder, kbBuilder2, kbBuilder3},
		[]test.Watcher{w, test.NewVersionWatcher(kibana2.KibanaVersionLabelName, opts...)},
	)
}

func TestVersionUpgradeToLatest8x(t *testing.T) {
	srcVersion := test.Ctx().ElasticStackVersion
	dstVersion := test.LatestSnapshotVersion8x

	test.SkipInvalidUpgrade(t, srcVersion, dstVersion)

	name := "test-version-upgrade-to-8x"
	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithVersion(srcVersion)

	srcNodeCount := 3
	kbBuilder := kibana.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(srcNodeCount).
		WithVersion(srcVersion)

	opts := []client.ListOption{
		client.InNamespace(kbBuilder.Kibana.Namespace),
		client.MatchingLabels(map[string]string{
			common.TypeLabelName:        kibana2.Type,
			kibana2.KibanaNameLabelName: kbBuilder.Kibana.Name,
		}),
	}

	// perform a Kibana version upgrade and assert that:
	// - there was a time were no Kibana pods were ready (when all old version pods were termintated,
	//   but before new version pods were started), and
	// - at all times all pods had the same Kibana version.
	test.RunMutationsWhileWatching(
		t,
		[]test.Builder{esBuilder, kbBuilder},
		[]test.Builder{
			esBuilder.WithVersion(dstVersion).WithMutatedFrom(&esBuilder),
			kbBuilder.WithVersion(dstVersion).WithMutatedFrom(&kbBuilder),
		},
		[]test.Watcher{NewReadinessWatcher(opts...), test.NewVersionWatcher(kibana2.KibanaVersionLabelName, opts...)},
	)
}

func TestVersionUpgradeAndRespecToLatest8x(t *testing.T) {
	srcVersion := test.Ctx().ElasticStackVersion
	dstVersion := test.LatestSnapshotVersion8x

	test.SkipInvalidUpgrade(t, srcVersion, dstVersion)

	name := "test-upgrade-and-respec-to-8x"
	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithVersion(dstVersion)

	srcNodeCount := 3

	kbBuilder1 := kibana.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(srcNodeCount).
		WithVersion(srcVersion)

	// perform a Kibana version upgrade immediately followed by a Kibana configuration change.
	// we want to make sure that the second upgrade will be done in rolling upgrade fashion instead of terminating
	// and recreating all the pods at once.
	kbBuilder2 := kbBuilder1.WithMutatedFrom(&kbBuilder1).WithVersion(dstVersion)
	kbBuilder3 := kbBuilder2.WithMutatedFrom(&kbBuilder2).WithLabel("some", "label")

	opts := []client.ListOption{
		client.InNamespace(kbBuilder1.Kibana.Namespace),
		client.MatchingLabels(map[string]string{
			common.TypeLabelName:        kibana2.Type,
			kibana2.KibanaNameLabelName: kbBuilder1.Kibana.Name,
		}),
	}

	// checks whether after temporary downtime kibana will be available the rest of the time
	var hadZero, shouldHaveNonZero, failed bool
	w := test.NewWatcher(
		"watch pods readiness: expect some downtime once and then no downtime",
		1*time.Second,
		func(k *test.K8sClient, t *testing.T) {
			pods, err := k.GetPods(opts...)
			if err != nil {
				t.Logf("got error: %v", err)
			}

			ready := len(reconcile.AvailableElasticsearchNodes(pods))
			hadZero = hadZero || ready == 0
			if hadZero && ready > 0 {
				shouldHaveNonZero = true
			}

			if shouldHaveNonZero && ready == 0 {
				failed = true
			}
		},
		func(k *test.K8sClient, t *testing.T) {
			assert.False(t, failed)
		})

	test.RunMutationsWhileWatching(
		t,
		[]test.Builder{esBuilder, kbBuilder1},
		[]test.Builder{esBuilder, kbBuilder2, kbBuilder3},
		[]test.Watcher{w, test.NewVersionWatcher(kibana2.KibanaVersionLabelName, opts...)},
	)
}

// NewReadinessWatcher returns a watcher that asserts that there was at least one observation where no matching pods
// were ready, ie. there was a period of unavailability. It relies on the assumption that pod termination and
// initialization take more than 1 second (observations resolution), so the said observation can't be missed.
func NewReadinessWatcher(opts ...client.ListOption) test.Watcher {
	var readinessObservations []int
	return test.NewWatcher(
		"watch pods readiness: expect some downtime",
		1*time.Second,
		func(k *test.K8sClient, t *testing.T) {
			if pods, err := k.GetPods(opts...); err != nil {
				t.Logf("failed to list pods: %v", err)
			} else {
				readinessObservations = append(readinessObservations, len(reconcile.AvailableElasticsearchNodes(pods)))
			}
		},
		func(k *test.K8sClient, t *testing.T) {
			assert.Contains(t, readinessObservations, 0)
		})
}

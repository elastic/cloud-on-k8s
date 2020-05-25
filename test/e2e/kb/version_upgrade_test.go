// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kb

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
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
	dstVersion := test.LatestVersion7x

	test.SkipInvalidUpgrade(t, srcVersion, srcVersion)

	name := "test-version-upgrade-to-7x"
	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithVersion(dstVersion)

	kbBuilder := kibana.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(3).
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
		[]test.Builder{esBuilder, kbBuilder.WithVersion(dstVersion).WithMutatedFrom(&kbBuilder)},
		[]test.Watcher{NewReadinessWatcher(opts...), NewVersionWatcher(opts...)},
	)
}

func TestVersionUpgradeAndRespecToLatest7x(t *testing.T) {
	srcVersion := test.Ctx().ElasticStackVersion
	dstVersion := test.LatestVersion7x

	test.SkipInvalidUpgrade(t, srcVersion, srcVersion)

	name := "test-upgrade-and-respec-to-7x"
	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithVersion(dstVersion)

	kbBuilder1 := kibana.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(3).
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
		[]test.Watcher{w, NewVersionWatcher(opts...)},
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

// NewVersionWatcher returns a watcher that asserts that in all observations all Kibana pods were running the same
// Kibana version. It relies on the assumption that pod initialization and termination take more than 1 second
// (observations resolution), so different versions running at the same time could always be caught.
func NewVersionWatcher(opts ...client.ListOption) test.Watcher {
	var podObservations [][]v1.Pod
	return test.NewWatcher(
		"watch pods versions: should not observe multiples versions running at once",
		1*time.Second,
		func(k *test.K8sClient, t *testing.T) {
			if pods, err := k.GetPods(opts...); err != nil {
				t.Logf("failed to list pods: %v", err)
			} else {
				podObservations = append(podObservations, pods)
			}
		},
		func(k *test.K8sClient, t *testing.T) {
			for _, pods := range podObservations {
				for i := 1; i < len(pods); i++ {
					assert.Equal(t, pods[i-1].Labels[kibana2.KibanaVersionLabelName], pods[i].Labels[kibana2.KibanaVersionLabelName])
				}
			}
		})
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kb

import (
	"testing"
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/kibana"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestVersionUpgrade(t *testing.T) {
	name := "test-version-upgrade"
	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithVersion("7.4.1")

	kbBuilder := kibana.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(3).
		WithVersion("7.4.0")

	ns := client.InNamespace(kbBuilder.Kibana.Namespace)
	matchLabels := client.MatchingLabels(map[string]string{
		common.TypeLabelName:      label.Type,
		label.KibanaNameLabelName: kbBuilder.Kibana.Name,
	})

	var observations []int
	w := test.NewWatcher("watch pods readiness", 1*time.Second,
		func(k *test.K8sClient, t *testing.T) {
			if pods, err := k.GetPods(ns, matchLabels); err != nil {
				t.Logf("got error: %v", err)
			} else {
				ready := 0
				for _, pod := range pods {
					if k8s.IsPodReady(pod) {
						ready++
					}
				}
				observations = append(observations, ready)
			}
		},
		func(k *test.K8sClient, t *testing.T) {
			assert.Contains(t, observations, 0)
		})

	test.RunMutationsWhileWatching(
		t,
		[]test.Builder{esBuilder, kbBuilder},
		[]test.Builder{esBuilder, kbBuilder.WithVersion("7.4.1").WithMutatedFrom(&kbBuilder)},
		[]test.Watcher{w},
	)
}

func TestRespecAndVersionUpgrade(t *testing.T) {
	name := "test-upgrade-and-respec"
	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithVersion("7.4.1")

	kbBuilder1 := kibana.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(3).
		WithVersion("7.4.0")

	kbBuilder2 := kbBuilder1.WithMutatedFrom(&kbBuilder1).WithVersion("7.4.1")
	kbBuilder3 := kbBuilder2.WithMutatedFrom(&kbBuilder2).WithLabel("some", "label")

	ns := client.InNamespace(kbBuilder1.Kibana.Namespace)
	matchLabels := client.MatchingLabels(map[string]string{
		common.TypeLabelName:      label.Type,
		label.KibanaNameLabelName: kbBuilder1.Kibana.Name,
	})

	// checks whether after temporary downtime kibana will be available the rest of the time
	var hadZero, shouldHaveNonZero, failed bool
	w := test.NewWatcher("watch pods readiness", 1*time.Second,
		func(k *test.K8sClient, t *testing.T) {
			pods, err := k.GetPods(ns, matchLabels)
			if err != nil {
				t.Logf("got error: %v", err)
			}

			ready := 0
			for _, pod := range pods {
				if k8s.IsPodReady(pod) {
					ready++
				}
			}

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
		[]test.Watcher{w},
	)
}

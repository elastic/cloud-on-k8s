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
	var errors []error
	w := test.NewWatcher("watch pods readiness", 1*time.Second,
		func(k *test.K8sClient, t *testing.T) {
			if pods, err := k.GetPods(ns, matchLabels); err != nil {
				errors = append(errors, err)
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

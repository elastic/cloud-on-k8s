// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
)

// NewChangeBudgetWatcher returns a watcher that checks whether the pod count stays within the given change budget.
// Assumes that observations resolution of 1 second is high enough to observe all changes steps.
func NewChangeBudgetWatcher(from esv1.ElasticsearchSpec, to esv1.Elasticsearch) test.Watcher {
	var PodCounts []int32
	var ReadyPodCounts []int32

	es := to.Spec
	//nolint:thelper
	return test.NewWatcher(
		"pod count for change budget: expect to stay within the change budget",
		1*time.Second,
		func(k *test.K8sClient, t *testing.T) {
			pods, err := sset.GetActualPodsForCluster(k.Client, to)
			if err != nil {
				t.Logf("got error listing pods: %v", err)
				return
			}
			podsReady := reconcile.AvailableElasticsearchNodes(pods)

			PodCounts = append(PodCounts, int32(len(pods)))
			ReadyPodCounts = append(ReadyPodCounts, int32(len(podsReady)))
		},
		func(k *test.K8sClient, t *testing.T) {
			desired := es.NodeCount()
			budget := es.UpdateStrategy.ChangeBudget

			// allowedMin, allowedMax bound observed values between the ones we expect to see given desired count and change budget.
			// seenMin, seenMax allow for ramping up/down nodes when moving from spec outside of <allowedMin, allowedMax> node count.
			// It's done by tracking lowest/highest values seen outside of bounds. This permits the values to only move monotonically
			// until they are inside <allowedMin, allowedMax>.
			maxSurge := budget.GetMaxSurgeOrDefault()
			if maxSurge != nil {
				allowedMax := desired + *maxSurge
				seenMin := from.NodeCount()
				for _, v := range PodCounts {
					if v <= allowedMax || v <= seenMin {
						seenMin = v
						continue
					}

					assert.Fail(t, "change budget violated", "pod count %d when allowed max was %d", v, allowedMax)
				}
			}

			maxUnavailable := budget.GetMaxUnavailableOrDefault()
			if maxUnavailable != nil {
				allowedMin := desired - *maxUnavailable
				seenMax := from.NodeCount()
				for _, v := range ReadyPodCounts {
					if v >= allowedMin || v >= seenMax {
						seenMax = v
						continue
					}

					assert.Fail(t, "change budget violated", "ready pod count %d when allowed min was %d", v, allowedMin)
				}
			}
		})
}

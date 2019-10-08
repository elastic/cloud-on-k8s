// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"fmt"
	"math"
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

type MasterChangeBudgetCheck struct {
	Observations []int
	Errors       []error
	stopChan     chan struct{}
	es           v1beta1.Elasticsearch
	interval     time.Duration
	client       k8s.Client
}

func NewMasterChangeBudgetCheck(es v1beta1.Elasticsearch, interval time.Duration, client k8s.Client) *MasterChangeBudgetCheck {
	return &MasterChangeBudgetCheck{
		es:       es,
		interval: interval,
		client:   client,
		stopChan: make(chan struct{}),
	}
}

func (mc *MasterChangeBudgetCheck) Start() {
	go func() {
		ticker := time.NewTicker(mc.interval)
		for {
			select {
			case <-mc.stopChan:
				return
			case <-ticker.C:
				pods, err := sset.GetActualMastersForCluster(mc.client, mc.es)
				if err != nil {
					mc.Errors = append(mc.Errors, err)
					continue
				}
				mc.Observations = append(mc.Observations, len(pods))
				continue
			}
		}
	}()
}

func (mc *MasterChangeBudgetCheck) Stop() {
	mc.stopChan <- struct{}{}
}

func (mc *MasterChangeBudgetCheck) Verify(maxRateOfChange int) error {
	for i := 1; i < len(mc.Observations); i++ {
		prev := mc.Observations[i-1]
		cur := mc.Observations[i]
		abs := int(math.Abs(float64(cur - prev)))
		if abs > maxRateOfChange {
			// This is ofc potentially flaky if we miss an observation and see suddenly more than one master
			// node popping up. This is due the fact that this check is depending on timing, the underlying
			// assumption being that the observation interval is always shorter than the time an Elasticsearch
			// node needs to boot.
			return fmt.Errorf("%d master changes in one observation, expected max %d", abs, maxRateOfChange)
		}
	}
	return nil
}

type ChangeBudgetCheck struct {
	PodCounts      []int32
	ReadyPodCounts []int32
	Errors         []error
	stopChan       chan struct{}
	es             v1beta1.Elasticsearch
	client         k8s.Client
}

func NewChangeBudgetCheck(es v1beta1.Elasticsearch, client k8s.Client) *ChangeBudgetCheck {
	return &ChangeBudgetCheck{
		es:       es,
		client:   client,
		stopChan: make(chan struct{}),
	}
}

func (c *ChangeBudgetCheck) Start() {
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		for {
			select {
			case <-c.stopChan:
				return
			case <-ticker.C:
				pods, err := sset.GetActualPodsForCluster(c.client, c.es)
				if err != nil {
					c.Errors = append(c.Errors, err)
					continue
				}
				podsReady := reconcile.AvailableElasticsearchNodes(pods)

				c.PodCounts = append(c.PodCounts, int32(len(pods)))
				c.ReadyPodCounts = append(c.ReadyPodCounts, int32(len(podsReady)))
			}
		}
	}()
}

func (c *ChangeBudgetCheck) Stop() {
	c.stopChan <- struct{}{}
}

func (c *ChangeBudgetCheck) Verify(from v1beta1.ElasticsearchSpec, to v1beta1.ElasticsearchSpec) error {
	desired := to.NodeCount()
	budget := to.UpdateStrategy.ChangeBudget

	// allowedMin, allowedMax bound observed values between the ones we expect to see given desired count and change budget.
	// seenMin, seenMax allow for ramping up/down nodes when moving from spec outside of <allowedMin, allowedMax> node count.
	// It's done by tracking lowest/highest values seen outside of bounds. This permits the values to only move monotonically
	// until they are inside <allowedMin, allowedMax>.
	maxSurge := budget.GetMaxSurgeOrDefault()
	if maxSurge != nil {
		allowedMax := desired + *maxSurge
		seenMin := from.NodeCount()
		for _, v := range c.PodCounts {
			if v <= allowedMax || v <= seenMin {
				seenMin = v
				continue
			}

			return fmt.Errorf("pod count %d when allowed max was %d", v, allowedMax)
		}
	}

	maxUnavailable := budget.GetMaxUnavailableOrDefault()
	if maxUnavailable != nil {
		allowedMin := desired - *maxUnavailable
		seenMax := from.NodeCount()
		for _, v := range c.ReadyPodCounts {
			if v >= allowedMin || v >= seenMax {
				seenMax = v
				continue
			}

			return fmt.Errorf("ready pod count %d when allowed min was %d", v, allowedMin)
		}
	}
	return nil
}

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
	PodCounts      []int
	ReadyPodCounts []int
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

				c.PodCounts = append(c.PodCounts, len(pods))
				c.ReadyPodCounts = append(c.PodCounts, len(podsReady))
			}
		}
	}()
}

func (c *ChangeBudgetCheck) Stop() {
	c.stopChan <- struct{}{}
}

func (c *ChangeBudgetCheck) Verify(esSpec v1beta1.ElasticsearchSpec) error {
	desired := int(esSpec.NodeCount())
	budget := esSpec.UpdateStrategy.ChangeBudget
	if budget == nil {
		budget = &v1beta1.ChangeBudget{MaxSurge: math.MaxInt32, MaxUnavailable: 1}
	}
	allowedMin := desired - budget.MaxUnavailable
	allowedMax := desired + budget.MaxSurge

	for _, v := range c.PodCounts {
		if v > allowedMax {
			return fmt.Errorf("pod count %d when allowed max was %d", v, allowedMax)
		}
	}
	for _, v := range c.ReadyPodCounts {
		if v < allowedMin {
			return fmt.Errorf("ready pod count %d when allowed min was %d", v, allowedMin)
		}
	}
	return nil
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"fmt"
	"math"
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

type MasterChangeBudgetCheck struct {
	Observations []int
	Errors       []error
	stopChan     chan struct{}
	es           v1alpha1.Elasticsearch
	interval     time.Duration
	client       k8s.Client
}

func NewMasterChangeBudgetCheck(es v1alpha1.Elasticsearch, interval time.Duration, client k8s.Client) *MasterChangeBudgetCheck {
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

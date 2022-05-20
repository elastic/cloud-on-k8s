// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var NOOPCheck func(k *K8sClient, t *testing.T)

type Watcher struct {
	name      string
	interval  time.Duration
	watchFn   func(k *K8sClient, t *testing.T)
	checkFn   func(k *K8sClient, t *testing.T)
	skipFn    func() bool
	stopChan  chan struct{}
	watchOnce bool
}

func NewWatcher(name string, interval time.Duration, watchFn func(k *K8sClient, t *testing.T), checkFn func(k *K8sClient, t *testing.T)) Watcher {
	return Watcher{
		name:      name,
		interval:  interval,
		watchFn:   watchFn,
		checkFn:   checkFn,
		stopChan:  make(chan struct{}),
		watchOnce: false,
	}
}

func NewConditionalWatcher(name string, interval time.Duration, watchFn func(k *K8sClient, t *testing.T), checkFn func(k *K8sClient, t *testing.T), skipFn func() bool) Watcher {
	watcher := NewWatcher(name, interval, watchFn, checkFn)
	watcher.skipFn = skipFn
	return watcher
}

func NewOnceWatcher(name string, watchFn func(k *K8sClient, t *testing.T), checkFn func(k *K8sClient, t *testing.T)) Watcher {
	return Watcher{
		name:      name,
		interval:  1 * time.Second,
		watchFn:   watchFn,
		checkFn:   checkFn,
		stopChan:  make(chan struct{}),
		watchOnce: true,
	}
}

func (w *Watcher) StartStep(k *K8sClient) Step {
	//nolint:thelper
	return Step{
		Name: fmt.Sprintf("Starting to %s", w.name),
		Test: func(t *testing.T) {
			if w.watchOnce {
				go w.watchFn(k, t)
				return
			}

			go func() {
				ticker := time.NewTicker(w.interval)
				for {
					select {
					case <-w.stopChan:
						return
					case <-ticker.C:
						w.watchFn(k, t)
					}
				}
			}()
		},
		Skip: w.skipFn,
	}
}

func (w *Watcher) StopStep(k *K8sClient) Step {
	//nolint:thelper
	return Step{
		Name: fmt.Sprintf("Stopping to %s", w.name),
		Test: func(t *testing.T) {
			w.stop()
			if w.checkFn != nil {
				w.checkFn(k, t)
			}
		},
		Skip: w.skipFn,
	}
}

func (w *Watcher) stop() {
	if !w.watchOnce {
		w.stopChan <- struct{}{}
	}
}

// NewVersionWatcher returns a watcher that asserts that in all observations all pods were running the same
// version. It relies on the assumption that pod initialization and termination take more than 1 second
// (observations resolution), so different versions running at the same time could always be caught.
func NewVersionWatcher(versionLabel string, opts ...client.ListOption) Watcher {
	var podObservations [][]v1.Pod
	return NewWatcher(
		"watch pods versions: should not observe multiples versions running at once",
		1*time.Second,
		func(k *K8sClient, t *testing.T) { //nolint:thelper
			if pods, err := k.GetPods(opts...); err != nil {
				t.Logf("failed to list pods: %v", err)
			} else {
				podObservations = append(podObservations, pods)
			}
		},
		func(k *K8sClient, t *testing.T) { //nolint:thelper
			for _, pods := range podObservations {
				for i := 1; i < len(pods); i++ {
					assert.Equal(t, pods[i-1].Labels[versionLabel], pods[i].Labels[versionLabel])
				}
			}
		})
}

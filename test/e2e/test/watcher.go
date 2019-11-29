// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package test

import (
	"fmt"
	"testing"
	"time"
)

type Watcher struct {
	name     string
	interval time.Duration
	watchFn  func(k *K8sClient, t *testing.T)
	checkFn  func(k *K8sClient, t *testing.T)
	stopChan chan struct{}
}

func NewWatcher(name string, interval time.Duration, watchFn func(k *K8sClient, t *testing.T), checkFn func(k *K8sClient, t *testing.T)) Watcher {
	return Watcher{
		name:     name,
		interval: interval,
		watchFn:  watchFn,
		checkFn:  checkFn,
		stopChan: make(chan struct{}),
	}
}

func (w *Watcher) StartStep(k *K8sClient) Step {
	return Step{
		Name: fmt.Sprintf("Starting to %s", w.name),
		Test: func(t *testing.T) {
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
		Skip: nil,
	}
}

func (w *Watcher) StopStep(k *K8sClient) Step {
	return Step{
		Name: fmt.Sprintf("Stopping to %s", w.name),
		Test: func(t *testing.T) {
			w.stop()
			w.checkFn(k, t)
		},
	}
}

func (w *Watcher) stop() {
	w.stopChan <- struct{}{}
}

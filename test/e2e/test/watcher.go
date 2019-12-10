// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package test

import (
	"fmt"
	"testing"
	"time"
)

var NOOPCheck func(k *K8sClient, t *testing.T) = nil

type Watcher struct {
	name      string
	interval  time.Duration
	watchFn   func(k *K8sClient, t *testing.T)
	checkFn   func(k *K8sClient, t *testing.T)
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
		Skip: nil,
	}
}

func (w *Watcher) StopStep(k *K8sClient) Step {
	return Step{
		Name: fmt.Sprintf("Stopping to %s", w.name),
		Test: func(t *testing.T) {
			w.stop()
			if w.checkFn != nil {
				w.checkFn(k, t)
			}
		},
	}
}

func (w *Watcher) stop() {
	if !w.watchOnce {
		w.stopChan <- struct{}{}
	}
}

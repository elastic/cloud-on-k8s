// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package scheduler

import (
	"sync"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// Scheduler allows to schedule generic reconciliation events on a channel.
type Scheduler interface {
	// Schedule an event for NamespacedName nsn after duration.
	Schedule(nsn types.NamespacedName, after time.Duration)
	// Events returns the channel onto which events are scheduled.
	Events() chan event.GenericEvent
}

type scheduler struct {
	timers map[types.NamespacedName]*time.Timer
	mutex  sync.Mutex
	out    chan event.GenericEvent
}

// NewScheduler creates a new scheduler.
func NewScheduler() Scheduler {
	return &scheduler{
		timers: map[types.NamespacedName]*time.Timer{},
		out:    make(chan event.GenericEvent),
	}
}

func (s *scheduler) Schedule(nsn types.NamespacedName, after time.Duration) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	timer, exists := s.timers[nsn]
	if exists {
		timer.Stop() // I think we don't need to drain the channel
	}
	s.timers[nsn] = time.AfterFunc(after, func() {
		s.out <- event.GenericEvent{
			Meta: &v1.ObjectMeta{
				Name:      nsn.Name,
				Namespace: nsn.Namespace,
			},
		}
	})
}

func (s *scheduler) Events() chan event.GenericEvent {
	return s.out
}

// Events returns a channel source for the events in Scheduler m.
func Events(m Scheduler) *source.Channel {
	return &source.Channel{
		Source: m.Events(),
	}
}

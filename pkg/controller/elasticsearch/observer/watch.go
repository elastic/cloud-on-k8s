// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package observer

import (
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// WatchClusterHealthChange returns a Source fed with generic events targeting clusters
// whose health has changed between 2 observations.
// Aimed to be used for triggering a reconciliation.
func WatchClusterHealthChange(m *Manager) *source.Channel {
	evtChan := make(chan event.GenericEvent)
	m.AddObservationListener(healthChangeListener(evtChan))
	return &source.Channel{
		// Each event in Source will be consumed and turned into
		// a reconciliation request.
		Source: evtChan,
		// DestBufferSize is kept at the default value (1024).
		// This means we can enqueue a maximum of 1024 requests
		// before blocking observers from moving on.
	}
}

// healthChangeListener returns an OnObservation listener that feeds a generic
// event when a cluster's observed health has changed.
func healthChangeListener(reconciliation chan event.GenericEvent) OnObservation {
	return func(cluster types.NamespacedName, previous State, new State) {
		// no-op if health hasn't change
		if !hasHealthChanged(previous, new) {
			return
		}

		// trigger a reconciliation event for that cluster
		evt := event.GenericEvent{
			Object: &esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{
				Namespace: cluster.Namespace,
				Name:      cluster.Name,
			}},
		}
		reconciliation <- evt
	}
}

// hasHealthChanged returns true if previous and new contain different health.
func hasHealthChanged(previous State, new State) bool {
	switch {
	// both nil
	case previous.ClusterHealth == nil && new.ClusterHealth == nil:
		return false
	// both equal
	case previous.ClusterHealth != nil && new.ClusterHealth != nil &&
		previous.ClusterHealth.Status == new.ClusterHealth.Status:
		return false
	// else: different
	default:
		return true
	}
}
